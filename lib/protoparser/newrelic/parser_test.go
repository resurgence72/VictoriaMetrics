package newrelic

import (
	"reflect"
	"strings"
	"testing"

	"github.com/valyala/fastjson"
)

func TestEvents_Unmarshal(t *testing.T) {
	tests := []struct {
		name    string
		metrics []Metric
		json    string
		wantErr bool
	}{
		{
			name:    "empty json",
			metrics: []Metric{},
			json:    "",
			wantErr: true,
		},
		{
			name: "json with correct data",
			metrics: []Metric{
				{
					Timestamp: 1690286061000,
					Tags: []Tag{
						{Key: "entity_key", Value: "macbook-pro.local"},
						{Key: "dc", Value: "1"},
					},
					Metric: "system_sample_disk_writes_per_second",
					Value:  0,
				},
				{
					Timestamp: 1690286061000,
					Tags: []Tag{
						{Key: "entity_key", Value: "macbook-pro.local"},
						{Key: "dc", Value: "1"},
					},
					Metric: "system_sample_uptime",
					Value:  762376,
				},
			},
			json: `[
    {
      "EntityID":28257883748326179,
      "IsAgent":true,
      "Events":[
        {
          "eventType":"SystemSample",
          "timestamp":1690286061,
          "entityKey":"macbook-pro.local",
		  "dc": "1",
          "diskWritesPerSecond":0,
          "uptime":762376
        }
      ],
      "ReportingAgentID":28257883748326179
    }
  ]`,
			wantErr: false,
		},
		{
			name:    "empty array in json",
			metrics: []Metric{},
			json:    `[]`,
			wantErr: false,
		},
		{
			name:    "empty events in json",
			metrics: []Metric{},
			json: `[
    {
      "EntityID":28257883748326179,
      "IsAgent":true,
      "Events":[],
      "ReportingAgentID":28257883748326179
    }
  ]`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Events{Metrics: []Metric{}}

			value, err := fastjson.Parse(tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("cannot parse json error: %s", err)
			}

			if value != nil {
				v, err := value.Array()
				if err != nil {
					t.Errorf("cannot get array from json")
				}
				if err := e.Unmarshal(v); (err != nil) != tt.wantErr {
					t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				}
				if !reflect.DeepEqual(e.Metrics, tt.metrics) {
					t.Errorf("got metrics => %v; expected = %v", e.Metrics, tt.metrics)
				}
			}
		})
	}
}

func Test_camelToSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want string
	}{
		{
			name: "empty string",
			str:  "",
			want: "",
		},
		{
			name: "lowercase all chars",
			str:  "somenewstring",
			want: "somenewstring",
		},
		{
			name: "first letter uppercase",
			str:  "Teststring",
			want: "teststring",
		},
		{
			name: "two uppercase letters",
			str:  "TestString",
			want: "test_string",
		},
		{
			name: "first and last uppercase letters",
			str:  "TeststrinG",
			want: "teststrin_g",
		},
		{
			name: "three letters uppercase",
			str:  "TestStrinG",
			want: "test_strin_g",
		},
		{
			name: "has many upper case letters",
			str:  "ProgressIOTime",
			want: "progress_io_time",
		},
		{
			name: "last all uppercase letters",
			str:  "ProgressTSDB",
			want: "progress_tsdb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := camelToSnakeCase(tt.str); got != tt.want {
				t.Errorf("camelToSnakeCase() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkCameToSnake(b *testing.B) {
	b.ReportAllocs()
	str := strings.Repeat("ProgressIOTime", 20)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			camelToSnakeCase(str)
		}
	})
}
