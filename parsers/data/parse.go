package data

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/NWACus/weather-telemetry/parsers/crbasic"
	"github.com/go-logr/logr"
)

func ParseAll(logger logr.Logger, files []string, tables map[int][]crbasic.DataTableConfiguration) error {
	for _, file := range files {
		fileLogger := logger.WithValues("file", file)
		fileLogger.Info("Parsing data file")
		contents, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", file, err)
		}
		raw, err := insertNewlines(contents)
		if err != nil {
			return fmt.Errorf("error while inserting newlines to %s: %v", file, err)
		}
		extents, err := distinguishExtents(raw)
		if err != nil {
			return fmt.Errorf("error while distinguishing extents in %s: %v", file, err)
		}
		fileLogger.Info("Parsed file into extents.", "extents", len(extents))
		for i, extent := range extents {
			extentLogger := fileLogger.WithValues("extent", i)
			table, year, err := interpret(extentLogger, extent, tables)
			if err != nil {
				return fmt.Errorf("error while interpreting extent %d in file %s: %v", i, file, err)
			}
			extentLogger = extentLogger.WithValues("year", year, "table", table.Name)
			extentLogger.Info("Associated extent with data table.")
		}
	}
	return nil
}

// interpret attempts to find the data table definition that matches the raw data rows.
// Consider the Blewett Pass data-set - we have raw logger data like:
//
//	96,2025,156,1100,13.13,89.2,77.6,30.93,-5.253,-1.236,0,0,3.39
//	96,2025,156,1200,13.08,96,81.8,19.02,-1.754,-0.583,0,0,3.39
//	96,2025,156,1300,13.02,83.6,15.94,0,-62.48,-0.563
//	96,2025,156,1400,13.02,85,14.91,0,-66.55,-0.83
//
// CRBasic configurations for the data tables exist for the 2021, 2022, 2024 and 2025 seasons.
// To correctly interpret the 2025,156,1200 row, we need to use the 2022 season configuration, as
// this data point is in the second half of the 2024 season, and the 2024 re-configuration is the
// one that took effect on the next row. The 2022 configuration was valid until that point.
//
// There's no obvious date at which the reconfigurations are known to have been applied, so the
// only reasonable algorithm that comes to mind right now is to start by looking at table configurations
// for the calendar year that the data point is from, walking backwards if no table definition
// matches the number of columns we're seeing.
func interpret(logger logr.Logger, rows [][]string, tables map[int][]crbasic.DataTableConfiguration) (crbasic.DataTableConfiguration, int, error) {
	if len(rows) == 0 {
		return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("extent contains no rows")
	}
	if len(rows[0]) < 5 {
		return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("expected at least 5 columns, got %d", len(rows[0]))
	}
	year, err := strconv.Atoi(rows[0][1])
	if err != nil {
		return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("could not parse year from record %s: %w", strings.Join(rows[0], ","), err)
	}
	columns := len(rows[0])

	var table *crbasic.DataTableConfiguration
	for {
		if year < 1980 {
			break
		}

		options, exist := tables[year]
		if !exist {
			year--
			continue
		}

		var tableOptions []string
		var candidates []crbasic.DataTableConfiguration
		for _, option := range options {
			tableOptions = append(tableOptions, fmt.Sprintf("%s (%d columns)", option.Name, len(option.DataLoggers)))
			if len(option.DataLoggers)+4 != columns {
				continue
			}

			modulus := option.Interval.Interval + option.Interval.Offset
			matches := true
			for _, row := range rows {
				timeStr := fmt.Sprintf("%04s", row[3])
				hours, err := strconv.Atoi(timeStr[0:2])
				if err != nil {
					return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("could not parse hours from row %s: %w", row, err)
				}
				minutes, err := strconv.Atoi(timeStr[2:4])
				if err != nil {
					return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("could not parse minutes from row %s: %w", row, err)
				}
				if (time.Duration(hours)*time.Hour+time.Duration(minutes)*time.Minute)%modulus != 0 {
					matches = false
					logger.Info("Skipping table, time doesn't match interval.", "time", timeStr, "interval", modulus.String())
					break
				}
			}

			if !matches {
				continue
			}

			candidates = append(candidates, option)
		}

		if len(candidates) == 0 {
			logger.V(4).Info("Found no table candidates in year.", "year", year)
			year--
			continue
		}

		if len(candidates) > 1 {
			return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("found %d candidates for %d columns", len(candidates), columns)
		}

		if len(candidates) == 1 {
			table = &candidates[0]
			break
		}
	}
	if table == nil {
		return crbasic.DataTableConfiguration{}, 0, fmt.Errorf("could not find a table for extent")
	}
	return *table, year, nil
}

// insertNewlines adds newlines to the raw data in order to arrive
// at a valid CSV: sometimes, the data files are missing a newline, like:
// 460,2007,243,2300,0,12.57,12.6,0,49.85,9.96,0.003,0
// 460,2007,244,0,0,12.55,12.57,0,49.5,9.92,0.003,0460,2007,244,100,0,12.52,12.55,0,48.55,9.47,0.003,0
// 460,2007,244,200,0,12.51,12.53,0,47.78,9.04,0.003,0
// as a heuristic, let's try parsing the first line of the file,
// figure out the first two columns (sensor ID and year), search the rest
// of the file for lines that are twice as long as they should be, that
// have that prefix somewhere in the middle, and split them
// TODO: handle multiple years?
func insertNewlines(raw []byte) ([]byte, error) {
	lines := bytes.Split(raw, []byte("\n"))
	firstRowCommas := bytes.Count(lines[0], []byte(","))
	firstRow, err := csv.NewReader(bytes.NewReader(lines[0])).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to parse first row: %w", err)
	}
	if len(firstRow) < 3 {
		return nil, fmt.Errorf("expected at least 3 rows, got %d", len(firstRow))
	}
	marker := []byte(strings.Join(firstRow[0:2], ",") + ",") // e.g. 460,2007,
	var amended [][]byte
	for _, line := range lines {
		var indices []int
		var offset int
		for offset < len(line) {
			index := bytes.Index(line[offset:], marker)
			if index == -1 {
				break
			}
			absoluteIndex := offset + index
			if bytes.Count(line[absoluteIndex:], []byte(",")) < firstRowCommas {
				break
			}
			indices = append(indices, absoluteIndex)
			offset += absoluteIndex + len(marker)
		}
		indices = append(indices, len(line))
		for i, index := range indices {
			if i < len(indices)-1 {
				amended = append(amended, line[index:indices[i+1]])
			}
		}
	}
	return bytes.Join(amended, []byte("\n")), nil
}

// distinguishExtents separates the raw CSV data from a .dat file into extents contiguous in time that
// share the same number of data fields - when a logger is re-configured during a season, we see a single
// .dat file with differing numbers of rows before and after the reconfiguration
// n.b. this is not sensitive to a re-ordering or other reconfiguration that does not change the overall
// number of columns
func distinguishExtents(raw []byte) ([][][]string, error) {
	reader := csv.NewReader(bytes.NewReader(raw))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV rows: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("data file contains no rows")
	}
	for i, row := range rows {
		if len(row) < 5 {
			return nil, fmt.Errorf("line %d: expected at least 5 columns, got %d", i, len(row))
		}
	}
	var parseErrorsWhileSorting []error
	slices.SortFunc(rows, func(a, b []string) int {
		aYear, err := strconv.Atoi(a[1])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		bYear, err := strconv.Atoi(b[1])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		if val := cmp.Compare(aYear, bYear); val != 0 { // year
			return val
		}

		aDay, err := strconv.Atoi(a[2])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		bDay, err := strconv.Atoi(b[2])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		if val := cmp.Compare(aDay, bDay); val != 0 { // day
			return val
		}

		aTime, err := strconv.Atoi(a[3])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		bTime, err := strconv.Atoi(b[3])
		if err != nil {
			parseErrorsWhileSorting = append(parseErrorsWhileSorting, err)
			return -1
		}
		return cmp.Compare(aTime, bTime) // time
	})
	if len(parseErrorsWhileSorting) > 0 {
		return nil, fmt.Errorf("failed to parse data in rows: %v", parseErrorsWhileSorting)
	}

	var extentIdx int
	extents := [][][]string{{}}
	for i := range rows {
		if i != 0 && len(rows[i]) != len(rows[i-1]) {
			extentIdx++
			extents = append(extents, [][]string{})
		}
		extents[extentIdx] = append(extents[extentIdx], rows[i])
	}
	return extents, nil
}
