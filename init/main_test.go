package main

import (
	"fmt"
	"testing"
)

func TestInit(t *testing.T) {

}

func TestNormFloat(t *testing.T) {
	for i := 0; i < 1000; i++ {
		val := normFloat(50, 25)
		fmt.Println(val)
		if val > 100 || val < 0 {
			t.Errorf("error, value should never be < 0 or > 100: %v", val)
		}
	}
}

func TestParseFloatInto(t *testing.T) {
	defaultValue := func() float64 { return 10.0 }

	var cases = []struct {
		input    string
		expected float64
	}{
		{"1", 1.0},
		{"-1", -1.0},
		{"0", 0.0},
		{"foo", defaultValue()},
	}

	for _, tt := range cases {
		val := defaultValue()
		parseFloatInto(&val, tt.input)
		if val != tt.expected {
			t.Errorf("parseFloatInto(%v): expected %v, actual %v", tt.input, tt.expected, val)
		}
	}
}
