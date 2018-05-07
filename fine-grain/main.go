// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

//// -------------------- Helper Methods -------------------- ////
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func FileAsWords(filename string) []string {
	bytes, err := ioutil.ReadFile(filename)
	check(err)
	return strings.Fields(string(bytes))
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

type pair struct {
	a, b string
}

//// -------------------- Prefix -------------------- ////
// Prefix is a Markov chain prefix of one or more words.
type Prefix []string

// String returns the Prefix as a string (for use as a map key).
func (p Prefix) String() string {
	return strings.Join(p, " ")
}

// Shift removes the first word from the Prefix and appends the given word.
func (p Prefix) Shift(word string) {
	copy(p, p[1:])
	p[len(p)-1] = word
}

//// -------------------- Chain -------------------- ////
// Chain contains a map ("chain") of prefixes to a list of suffixes.
// A prefix is a string of prefixLen words joined with spaces.
// A suffix is a single word. A prefix can have multiple suffixes.
type Chain struct {
	chain     sync.Map
	prefixLen int
}

// NewChain returns a new Chain with prefixes of prefixLen words.
func NewChain(prefixLen int) *Chain {
	return &Chain{sync.Map{}, prefixLen}
}

func (c *Chain) Insert(key, word string) {
	values, _ := c.chain.Load(key)
	notStored := true
	for notStored {
		if values == nil {
			values = append(make([]string, 0), word)
		} else {
			values = append(values.([]string), word)
		}
		c.chain.Delete(key)
		values, notStored = c.chain.LoadOrStore(key, values)
	}
}

//////// -------------------- BUILD -------------------- ////////
func (c *Chain) Build(words []string, workers, inserters int) {
	switch workers {
	case 1:
		// Simple sequential implementation
		p := make(Prefix, c.prefixLen)
		for _, word := range words {
			key := p.String()
			c.Insert(key, word)
			p.Shift(word)
		}
	case 0:
		c.BuildGo(words, inserters)
	default:
		c.BuildTP(words, workers, inserters)
	}
}

//// -------------------- Goroutines -------------------- ////
// Spawns goroutines per prefix-word pair
func (c *Chain) BuildGo(words []string, inserters int) {
	switch inserters {
	case 0:
		c.BuildGoLock(words) // Fine-grain locks
	case 1:
		c.BuildGoChannel(words) // One channel
	default:
		c.BuildGoChannels(words, inserters) // "inserters" channels
	}
}

func (c *Chain) BuildGoLock(words []string) {
	var wg sync.WaitGroup

	// Take care of first edge cases
	p := make(Prefix, c.prefixLen)
	for _, word := range words[:c.prefixLen] {
		key := p.String()
		c.Insert(key, word)
		p.Shift(word)
	}

	for idx := c.prefixLen; idx < len(words); idx++ {
		wg.Add(1)
		// Do work
		go func(words []string, idx int) {
			key := strings.Join(words[idx-c.prefixLen:idx], " ")
			c.Insert(key, words[idx])
			wg.Done()
		}(words, idx)
	}

	wg.Wait()
}

func (c *Chain) BuildGoChannel(words []string) {
	ch := make(chan pair, 8)
	var wg sync.WaitGroup

	// Take care of first edge cases
	p := make(Prefix, c.prefixLen)
	for _, word := range words[:c.prefixLen] {
		key := p.String()
		ch <- pair{key, word}
		p.Shift(word)
	}

	for idx := c.prefixLen; idx < len(words); idx++ {
		wg.Add(1)
		// Do work
		go func(words []string, idx int) {
			key := strings.Join(words[idx-c.prefixLen:idx], " ")
			ch <- pair{key, words[idx]}
			wg.Done()
		}(words, idx)
	}

	go func() {
		for p := range ch {
			key, word := p.a, p.b
			c.Insert(key, word)
		}
		wg.Done()
	}()

	wg.Wait()
	wg.Add(1)
	close(ch)
	wg.Wait()
}

func (c *Chain) BuildGoChannels(words []string, inserters int) {
	ch := make(chan pair, 8)
	var wg sync.WaitGroup

	// Take care of first edge cases
	p := make(Prefix, c.prefixLen)
	for _, word := range words[:c.prefixLen] {
		key := p.String()
		ch <- pair{key, word}
		p.Shift(word)
	}

	for idx := c.prefixLen; idx < len(words); idx++ {
		wg.Add(1)
		// Do work
		go func(words []string, idx int) {
			key := strings.Join(words[idx-c.prefixLen:idx], " ")
			ch <- pair{key, words[idx]}
			wg.Done()
		}(words, idx)
	}

	for i := 0; i < inserters; i++ {
		go func() {
			for p := range ch {
				key, word := p.a, p.b
				c.Insert(key, word)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	wg.Add(inserters)
	close(ch)
	wg.Wait()
}

//// -------------------- Thread-Pool -------------------- ////
// Evenly partitions text between workers (so not really a thread pool)
func (c *Chain) BuildTP(words []string, workers, inserters int) {
	switch inserters {
	case 0:
		c.BuildTPLock(words, workers) // fine-grain lock
	case 1:
		c.BuildTPChannel(words, workers) // One channel
	default:
		c.BuildTPChannels(words, workers, inserters) // "inserters" channels
	}
}

func (c *Chain) BuildTPLock(words []string, workers int) {
	var wg sync.WaitGroup
	wg.Add(workers)

	perWorker := len(words) / workers
	if len(words)%workers != 0 {
		perWorker++
	}
	for i := 0; i < workers; i++ {
		start := perWorker * i
		end := min(perWorker*(i+1), len(words))
		go func(words []string, start, end int) {
			// Initialize Prefix
			p := make(Prefix, c.prefixLen)
			if start != 0 {
				for idx := 0; idx < c.prefixLen; idx++ {
					// There is a dumb case where this Index Out of Bounds
					p[idx] = words[start+idx-c.prefixLen]
				}
			}

			// Do work
			for _, word := range words[start:end] {
				key := p.String()
				c.Insert(key, word)
				p.Shift(word)
			}
			wg.Done()
		}(words, start, end)
	}

	wg.Wait()
}

func (c *Chain) BuildTPChannel(words []string, workers int) {
	ch := make(chan pair, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	perWorker := len(words) / workers
	if len(words)%workers != 0 {
		perWorker++
	}
	for i := 0; i < workers; i++ {
		start := perWorker * i
		end := min(perWorker*(i+1), len(words))
		go func(words []string, start, end int) {
			// Initialize Prefix
			p := make(Prefix, c.prefixLen)
			if start != 0 {
				for idx := 0; idx < c.prefixLen; idx++ {
					// There is a dumb case where this Index Out of Bounds
					p[idx] = words[start+idx-c.prefixLen]
				}
			}

			// Do work
			for _, word := range words[start:end] {
				key := p.String()
				ch <- pair{key, word}
				p.Shift(word)
			}
			wg.Done()
		}(words, start, end)
	}

	go func() {
		for p := range ch {
			key, word := p.a, p.b
			c.Insert(key, word)
		}
		wg.Done()
	}()

	wg.Wait()
	wg.Add(1)
	close(ch)
	wg.Wait()
}

func (c *Chain) BuildTPChannels(words []string, workers, inserters int) {
	ch := make(chan pair, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	perWorker := len(words) / workers
	if len(words)%workers != 0 {
		perWorker++
	}
	for i := 0; i < workers; i++ {
		start := perWorker * i
		end := min(perWorker*(i+1), len(words))
		go func(words []string, start, end int) {
			// Initialize Prefix
			p := make(Prefix, c.prefixLen)
			if start != 0 {
				for idx := 0; idx < c.prefixLen; idx++ {
					// There is a dumb case where this Index Out of Bounds
					p[idx] = words[start+idx-c.prefixLen]
				}
			}

			// Do work
			for _, word := range words[start:end] {
				key := p.String()
				ch <- pair{key, word}
				p.Shift(word)
			}
			wg.Done()
		}(words, start, end)
	}

	for i := 0; i < inserters; i++ {
		go func() {
			for p := range ch {
				key, word := p.a, p.b
				c.Insert(key, word)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	wg.Add(inserters)
	close(ch)
	wg.Wait()
}

// Generate returns a string of at most n words generated from Chain.
func (c *Chain) Generate(n int) string {
	p := make(Prefix, c.prefixLen)
	var words []string
	for i := 0; i < n; i++ {
		choicesObj, _ := c.chain.Load(p.String())
		if choicesObj == nil {
			break
		}
		choices := ([]string)(choicesObj.([]string))
		next := choices[rand.Intn(len(choices))]
		words = append(words, next)
		p.Shift(next)
	}
	return strings.Join(words, " ")
}

func main() {
	// Register command-line flags.
	numWords := flag.Int("words", 100, "maximum number of words to print")
	prefixLen := flag.Int("prefix", 2, "prefix length in words")

	workers := flag.Int("workers", 1, "Number of workers")
	inserters := flag.Int("inserters", 0, "Number of inserters")

	inputFile := flag.String("input", "", "Input file path")
	// outputFile := flag.String("output", "out.txt", "Output file path")

	// Set up output
	// output, err := os.Open(*outputFile)
	// check(err)
	dataFile, err := os.OpenFile("data.csv", os.O_APPEND|os.O_WRONLY, 0644)
	check(err)
	// defer output.Close()
	defer dataFile.Close()
	flag.Parse()                     // Parse command-line flags.
	rand.Seed(time.Now().UnixNano()) // Seed the random number generator.

	words := FileAsWords(*inputFile)
	c := NewChain(*prefixLen) // Initialize a new Chain.

	debug.SetGCPercent(-1)
	runtime.GC()

	///// Where the magic happens /////
	start := time.Now()
	c.Build(words, *workers, *inserters)
	elapsed := time.Since(start)
	///// Where the magic happens /////

	fmt.Fprintf(dataFile, "%.6f,", elapsed.Seconds())

	fmt.Println(c.Generate(*numWords)) // Generate text.
	// fmt.Fprintln(output, text)    // Write text to standard output.
}
