package crbasic

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
)

// DataTableConfiguration reflects the data loggers which are configured to make up the .dat data archives
// from a weather station. We parse the .cr1 CRBasic configuration program to determine the quantities
// being measured, their units, and the order in which they will appear in the data source.
type DataTableConfiguration struct {
	// Name is the name given to the DataTable declaration.
	Name string
	// Interval configures when the data table is evaluated.
	Interval Interval
	// DataLoggers
	DataLoggers []DataLogger
}

// DataLogger is a piece of equipment measuring some atmospheric quantity, the data from which end up as
// a column in the .dat data archives from a weather station.
type DataLogger struct {
	// Name is the variable name given to this data logger in the CRBasic file.
	Name string

	// Unit is a normalized (lower-case) human-readable name for the units used by this logger.
	Unit string

	// Method is the data acquisition method for this logger - for example, an average, a sample, or a maximum.
	Method string

	// DataType is the type of data output by this data logger.
	DataType DataType
}

// DataType determines the type of data output by a data logger.
type DataType string

const (
	DataTypeIEEE4   DataType = "IEEE4"   // IEEE four-byte floating point (IEEE 754 std). IEEE4 can also be specified using the keyword FLOAT.
	DataTypeFP2     DataType = "FP2"     // Campbell Scientific two-byte floating point.
	DataTypeIEEE8   DataType = "IEEE8"   // IEEE eight-byte floating point (double)
	DataTypeString  DataType = "STRING"  // ASCII string; size defined by program
	DataTypeBoolean DataType = "BOOLEAN" // 0 = False; -1 = True
	DataTypeBOOL8   DataType = "BOOL8"   // 1-byte Boolean value
	DataTypeLong    DataType = "LONG"    // 32-bit long integer, ranging from -2,147,483,648 to +2,147,483,647
	DataTypeNSEC    DataType = "NSEC"    // 8-byte time stamp format
	DataTypeUINT1   DataType = "UINT1"   // One-byte unsigned integer
	DataTypeUINT2   DataType = "UINT2"   // Two-byte unsigned integer
	DataTypeUINT4   DataType = "UINT4"   // Four-byte unsigned integer
)

type Interval struct {
	// Interval is how frequently data will be stored to the data table. If the value here is 1 hour, for instance,
	// data will be stored every time the real-time clock in the logger transitions to a new hour (e.g., at the hour).
	Interval time.Duration

	// Offset is how long after the interval trigger to record data. If the value for interval is 1 hour and offset is
	// 15 minutes, data will be stored at 15 minutes after the hour.
	Offset time.Duration
}

// TODO: how do we know which table is creating the data?

// Parse finds all data table configurations in the CRBasic file.
func Parse(input []byte) ([]DataTableConfiguration, error) {
	var tables []DataTableConfiguration

	lines := bytes.Split(input, []byte("\n"))

	declarationIndex := -1
	for i := 0; i < len(lines); i++ {
		line := trimComment(lines[i])
		if bytes.Contains(line, []byte("DataTable")) {
			if declarationIndex != -1 {
				return nil, fmt.Errorf("encountered DataTable declaration on line %d, without finding EndTable for previous declaration on %d", i, declarationIndex)
			}
			declarationIndex = i
			continue
		}
		if bytes.Contains(line, []byte("EndTable")) {
			if declarationIndex == -1 {
				return nil, fmt.Errorf("found EndTable declaration on line %d without any DataTable declaration before it", i)
			}
			table, err := parseTable(lines[declarationIndex : i+1])
			if err != nil {
				fmt.Println(string(bytes.Join(lines[declarationIndex:i+1], []byte("\n"))))
				return nil, fmt.Errorf("failed to parse table declaration from lines %d:%d: %w", declarationIndex, i, err)
			}
			tables = append(tables, table)
			declarationIndex = -1
			continue
		}
	}
	if declarationIndex != -1 {
		return nil, fmt.Errorf("failed to find EndTable declaration before document end for DataTable declaration on line %d", declarationIndex)
	}

	return tables, nil
}

func parseTable(input [][]byte) (DataTableConfiguration, error) {
	if len(input) < 3 {
		return DataTableConfiguration{}, fmt.Errorf("expected at least 3 entries in data table declaration, got %d: %s", len(input), string(bytes.Join(input, []byte("\n"))))
	}
	dataTableMethod, dataTableArgs, err := parseCall(trimComment(input[0]))
	if err != nil {
		return DataTableConfiguration{}, fmt.Errorf("failed to parse statement: %w", err)
	}
	if !bytes.Equal(dataTableMethod, []byte("DataTable")) {
		return DataTableConfiguration{}, fmt.Errorf("failed to find DataTable declaration on first line, got %s: %s", string(dataTableMethod), input[0])
	}
	tableName := dataTableArgs[0]

	var interval *Interval
	var loggers []DataLogger
	for _, line := range input[1 : len(input)-1] {
		l := bytes.TrimSpace(trimComment(line))
		if len(l) == 0 {
			continue
		}
		method, args, err := parseCall(l)
		if err != nil {
			return DataTableConfiguration{}, fmt.Errorf("failed to parse statement: %s: %w", string(line), err)
		}
		if bytes.Equal(method, []byte(`FieldNames`)) || bytes.Equal(method, []byte(`OpenInterval`)) || bytes.Equal(method, []byte(`DataTime`)) {
			continue
		}
		if bytes.Equal(bytes.ToLower(method), []byte(`datainterval`)) {
			if len(args) != 4 {
				return DataTableConfiguration{}, fmt.Errorf("expected 4 args for DataInterval, got %d: %s", len(args), string(line))
			}
			timeUnit, err := parseTimeUnit(args[2])
			if err != nil {
				return DataTableConfiguration{}, fmt.Errorf("failed to parse timeUnit declaration in DataInterval: %s", input[0])
			}
			intervalVal, err := strconv.ParseInt(string(args[1]), 10, 64)
			if err != nil {
				return DataTableConfiguration{}, fmt.Errorf("failed to parse interval declaration in DataInterval: %s", input[0])
			}
			offsetVal, err := strconv.ParseInt(string(args[0]), 10, 64)
			if err != nil {
				return DataTableConfiguration{}, fmt.Errorf("failed to parse offset declaration in DataInterval: %s", input[0])
			}
			interval = &Interval{
				Interval: time.Duration(intervalVal) * timeUnit,
				Offset:   time.Duration(offsetVal) * timeUnit,
			}
			continue
		}
		dataTypeIndex := 2
		switch string(method) {
		case "WindVector", "Median":
			dataTypeIndex = 3
		}
		if len(args) < dataTypeIndex+1 {
			return DataTableConfiguration{}, fmt.Errorf("expected at least %d args, got %d: %s", dataTypeIndex+1, len(args), string(line))
		}
		dataType, err := parseDataType(args[dataTypeIndex])
		if err != nil {
			return DataTableConfiguration{}, fmt.Errorf("failed to parse statement: %s: %w", string(line), err)
		}
		loggers = append(loggers, DataLogger{
			Name:     string(args[1]), // TODO: confirm this makes sense? windvector not so much
			Unit:     "",              // TODO:
			Method:   string(method),
			DataType: dataType,
		})
	}

	if interval == nil {
		return DataTableConfiguration{}, fmt.Errorf("could not find DataInterval declaration in DataTable: %s", string(bytes.Join(input, []byte("\n"))))
	}

	return DataTableConfiguration{
		Name:        string(tableName),
		Interval:    *interval,
		DataLoggers: loggers,
	}, nil
}

// CRBasic comments begin with '
func trimComment(input []byte) []byte {
	commentIndex := bytes.IndexRune(input, '\'')
	if commentIndex == -1 {
		commentIndex = len(input)
	}
	return input[:commentIndex]
}

// parseCall parses a call like 'Method(arg1, arg2, arg3)' into the method name and arguments.
func parseCall(input []byte) ([]byte, [][]byte, error) {
	if bytes.Equal(bytes.TrimSpace(input), []byte("OpenInterval")) {
		return []byte("OpenInterval"), nil, nil
	}
	openParen := bytes.Index(input, []byte("("))
	if openParen == -1 {
		return nil, nil, fmt.Errorf("could not find ( in input: %s", string(input))
	}
	closeParen := bytes.LastIndex(input, []byte(")")) // n.b. we really need a full parser, but whatever
	if closeParen == -1 {
		// hack for now, we seem to be missing many
		closeParen = len(input)
		//return nil, nil, fmt.Errorf("could not find ) in input: %s", string(input))
	}
	method := bytes.TrimSpace(input[:openParen])
	rawArgs := bytes.Split(input[openParen+1:closeParen], []byte(","))
	args := make([][]byte, len(rawArgs))
	for i, arg := range rawArgs {
		args[i] = bytes.TrimSpace(arg)
	}

	return method, args, nil
}

func parseTimeUnit(input []byte) (time.Duration, error) {
	switch string(bytes.ToLower(input)) {
	case `msec`:
		return 1 * time.Millisecond, nil
	case `sec`:
		return 1 * time.Second, nil
	case `min`:
		return 1 * time.Minute, nil
	case `hr`:
		return 1 * time.Hour, nil
	case `day`:
		return 24 * time.Hour, nil
	case `mon`:
		return 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown time unit: %s", string(input))
	}
}

func parseDataType(input []byte) (DataType, error) {
	switch t := DataType(bytes.ToUpper(input)); t {
	case DataTypeIEEE4, DataTypeFP2, DataTypeIEEE8, DataTypeString, DataTypeBoolean, DataTypeBOOL8, DataTypeLong, DataTypeNSEC, DataTypeUINT1, DataTypeUINT2, DataTypeUINT4:
		return t, nil
	default:
		return "", fmt.Errorf("unknown data type: %s", string(input))
	}
}
