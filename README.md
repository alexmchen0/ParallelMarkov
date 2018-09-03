# Parallel Markov Chain Builder

A random text generator using a parallelized version of the Markov chain algorithm implemented [here](https://golang.org/doc/codewalk/markov/).

Explores Go's extensive concurrency primitives (goroutines, channels, mutexes, barriers, etc.) and synchronization techniques with a goal to achieve speedup in a pretty fun/interesting application.

Goal: To parallelize Markov chain creation for random word generators in Go. The Markov chain is stored as a map from prefixes to suffixes which is built upon/written to by multiple threads and thus needs to be synchronized.
- `coarse-grain` compares synchronization of a shared `map[string][]string` using [`sync.Mutex`](https://golang.org/pkg/sync/#Mutex) vs [channels](https://tour.golang.org/concurrency/2)
- `fine-grain` stores the Markov chain in a [`sync.Map`](https://golang.org/pkg/sync/#Map), Go's concurrent map implementation
