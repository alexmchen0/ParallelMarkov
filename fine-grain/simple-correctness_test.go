package main

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
)

var correct = map[string][]string{
	" ":         {"I"},
	" I":        {"am"},
	"I am":      {"a", "not"},
	"a free":    {"man!"},
	"am a":      {"free"},
	"am not":    {"a"},
	"a number!": {"I"},
	"number! I": {"am"},
	"not a":     {"number!"},
}

func printMap(m map[string][]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		v := m[k]
		fmt.Printf("'%s': %v\t", k, v)
	}
	fmt.Println()
}

func makeMap(sm sync.Map) map[string][]string {
	m := make(map[string][]string)
	sm.Range(func(k, v interface{}) bool {
		m[k.(string)] = v.([]string)
		return true
	})
	return m
}

func Equal(m1, m2 map[string][]string) bool {
	for k, v1 := range m1 {
		v2 := m2[k]
		sort.Slice(v1, func(i, j int) bool { return v1[i] < v1[j] })
		sort.Slice(v2, func(i, j int) bool { return v2[i] < v2[j] })
	}

	return reflect.DeepEqual(m1, m2)
}

func RunTest(t *testing.T, workers, inserters int) {
	prefixLen := 2
	words := FileAsWords("simple.txt")
	c := NewChain(prefixLen)
	c.Build(words, workers, inserters)
	cmap := makeMap(c.chain)
	if !Equal(cmap, correct) {
		fmt.Printf("Fail with %d workers, %d inserters\n", workers, inserters)
		printMap(cmap)
		printMap(correct)
		t.Fail()
	}
}

func TestSimple(t *testing.T) {
	RunTest(t, 1, 0)
}

func TestSimpleGoLock(t *testing.T) {
	RunTest(t, 0, 0)
}

func TestSimpleGoChannel(t *testing.T) {
	RunTest(t, 0, 1)
}

func TestSimpleGoChannels(t *testing.T) {
	RunTest(t, 0, 4)
}

func TestSimpleTPLock(t *testing.T) {
	RunTest(t, 4, 0)
}

func TestSimpleTPChannel(t *testing.T) {
	RunTest(t, 4, 1)
}

func TestSimpleTPChannels(t *testing.T) {
	RunTest(t, 4, 4)
}
