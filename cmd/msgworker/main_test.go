package main

import (
	"reflect"
	"testing"
)

func TestParseBrokerList(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "single", raw: "kafka:19092", want: []string{"kafka:19092"}},
		{name: "multiple", raw: "kafka1:9092,kafka2:9092", want: []string{"kafka1:9092", "kafka2:9092"}},
		{name: "spaces trimmed", raw: " kafka1:9092 , kafka2:9092 ", want: []string{"kafka1:9092", "kafka2:9092"}},
		{name: "empty entries dropped", raw: "kafka1:9092,,kafka2:9092,", want: []string{"kafka1:9092", "kafka2:9092"}},
		{name: "only commas", raw: ",,,", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBrokerList(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseBrokerList(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
