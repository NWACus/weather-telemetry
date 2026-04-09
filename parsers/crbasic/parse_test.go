package crbasic

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParseCall(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		input       string
		method      string
		args        []string
		expectedErr bool
	}{
		{
			name:   "normal case",
			input:  `DataTable(Table60, true, -1)`,
			method: `DataTable`,
			args:   []string{`Table60`, `true`, `-1`},
		},
		{
			name:   "whitespace",
			input:  `  DataTable  (  Table60  , true, -1)`,
			method: `DataTable`,
			args:   []string{`Table60`, `true`, `-1`},
		},
		{
			name:   "many args",
			input:  `WindVector(1, WindSpeed, Wind_Dir, FP2, 0, 0, 0, 3)`,
			method: `WindVector`,
			args:   []string{`1`, `WindSpeed`, `Wind_Dir`, `FP2`, `0`, `0`, `0`, `3`},
		},
		{
			name:        "no opening parens",
			input:       `WindVector1, WindSpeed, Wind_Dir, FP2, 0, 0, 0, 3)`,
			expectedErr: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			method, args, err := parseCall([]byte(testCase.input))
			if (err != nil) != testCase.expectedErr {
				t.Errorf("parseCall() error = %v, wantErr %v", err, testCase.expectedErr)
			}
			if testCase.expectedErr {
				return
			}
			if diff := cmp.Diff(string(method), testCase.method); diff != "" {
				t.Errorf("parseCall() returned unexpected method (-want +got):\n%s", diff)
			}
			stringArgs := make([]string, len(args))
			for i, arg := range args {
				stringArgs[i] = string(arg)
			}
			if diff := cmp.Diff(stringArgs, testCase.args); diff != "" {
				t.Errorf("parseCall() returned unexpected args (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseTable(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		input       string
		table       DataTableConfiguration
		expectedErr bool
	}{
		{
			name: "normal case",
			input: `DataTable(Table15, true, -1)
  DataInterval(0, 15, Min, 10)
  Average(1, Batt_Volt, FP2, 0)
  Sample(1, AirTemp_Avg, FP2)
  Sample(1, RH_Avg, FP2)
  Minimum(1, WindSpeed, FP2, 0, False)
  Average(1, WindSpeed, FP2, 0)
  Maximum(1, WindSpeed, FP2, 0, False)
  WindVector(1, WindSpeed, Wind_Dir, FP2, 0, 0, 0, 3)
  Median(1, HN_depth, 12 , FP2, StoreDepth)
EndTable`,
			table: DataTableConfiguration{
				Name: "Table15",
				Interval: Interval{
					Interval: 15 * time.Minute,
					Offset:   0,
				},
				DataLoggers: []DataLogger{
					{Name: "Batt_Volt", Method: "Average", DataType: "FP2"},
					{Name: "AirTemp_Avg", Method: "Sample", DataType: "FP2"},
					{Name: "RH_Avg", Method: "Sample", DataType: "FP2"},
					{Name: "WindSpeed", Method: "Minimum", DataType: "FP2"},
					{Name: "WindSpeed", Method: "Average", DataType: "FP2"},
					{Name: "WindSpeed", Method: "Maximum", DataType: "FP2"},
					{Name: "WindSpeed", Method: "WindVector", DataType: "FP2"},
					{Name: "HN_depth", Method: "Median", DataType: "FP2"},
				},
			},
		},
		{
			name: "normal case",
			input: `DataTable(GillWind15, true, -1)
  DataInterval(0, 15, Min, 10)
  Minimum(1, Gill_spd, FP2, BadWind, False) 'Average(1, Gill_spd, FP2, BadWind)
  Average(1, Gill_spd, FP2, BadWind)
  Maximum(1, Gill_spd, FP2, BadWind, False)
  WindVector(1, Gill_spd, Gill_dir, FP2, BadWind, 0, 0, 3)
  Sample(1, HrCount, Long)  'OUTPUT GILL NUMBER OF RECORDS RECORDED
EndTable`,
			table: DataTableConfiguration{
				Name: "GillWind15",
				Interval: Interval{
					Interval: 15 * time.Minute,
					Offset:   0,
				},
				DataLoggers: []DataLogger{
					{Name: "Gill_spd", Method: "Minimum", DataType: "FP2"},
					{Name: "Gill_spd", Method: "Average", DataType: "FP2"},
					{Name: "Gill_spd", Method: "Maximum", DataType: "FP2"},
					{Name: "Gill_spd", Method: "WindVector", DataType: "FP2"},
					{Name: "HrCount", Method: "Sample", DataType: "LONG"},
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			table, err := parseTable(bytes.Split([]byte(testCase.input), []byte("\n")))
			if (err != nil) != testCase.expectedErr {
				t.Errorf("parseCall() error = %v, wantErr %v", err, testCase.expectedErr)
			}
			if testCase.expectedErr {
				return
			}
			if diff := cmp.Diff(table, testCase.table); diff != "" {
				t.Errorf("parseTable() returned unexpected table (-want +got):\n%s", diff)
			}
		})
	}
}
