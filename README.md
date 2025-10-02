# rediscache

[![Go Report Card](https://goreportcard.com/badge/github.com/chi07/rediscache)](https://goreportcard.com/report/github.com/chi07/rediscache)
[![codecov](https://codecov.io/gh/chi07/rediscache/branch/main/graph/badge.svg)](https://codecov.io/gh/chi07/rediscache)
[![CI](https://github.com/chi07/rediscache/actions/workflows/ci.yml/badge.svg)](https://github.com/chi07/rediscache/actions/workflows/ci.yml)

**rediscache** is a **thread-safe circular buffer (ring buffer) cache** with a fixed capacity inspired by original [rediscache](https://github.com/hadv/rediscache) by HaDV.
It stores `(key, value)` pairs up to a predefined limit. When the cache is full, the oldest entry is overwritten (evicted).  
You can attach an `EvictCallback` to get notified whenever an item is evicted.

## Features

- 🚀 **Thread-safe** (using `sync.RWMutex`)
- ♻️ **Fixed-size circular buffer** (bounded memory usage)
- 🔔 **Evict callback** for custom eviction handling
- ⚡ **O(1) Push/Load/Delete**
- ✨ Simple, idiomatic Go API with generics

## Installation

```bash
go get github.com/chi07/rediscache
```
Usage

1. Basic Example
```go
package main

import (
    "fmt"
    "github.com/chi07/rediscache"
)

func main() {
    // Create a cache with capacity=3, key=int, value=string
    rc, err := rediscache.New[int, string](3)
    if err != nil {
        panic(err)
    }

    rc.Push(1, "one")
    rc.Push(2, "two")

    // Load a value
    if v, ok := rc.Load(1); ok {
        fmt.Println("Key 1 =", v) // "one"
    }

    fmt.Println("Size:", rc.Size())       // 2
    fmt.Println("Capacity:", rc.Capacity()) // 3
}
```

2. With Eviction Callback
```go
rc, _ := rediscache.NewWithEvictCallback[int, string](2, func(k int, v string) {
    fmt.Printf("Evicted key=%d, value=%s\n", k, v)
})

rc.Push(1, "one")
rc.Push(2, "two")

// Adding another element evicts "1"
rc.Push(3, "three")
// Output: Evicted key=1, value=one
```

### 3. API Overview

- **`New[K, V](capacity int) (*rediscache[K, V], error)`**  
  Creates a new cache with the given capacity.

- **`NewWithEvictCallback[K, V](capacity int, cb EvictCallback[K, V])`**  
  Creates a new cache with an eviction callback.

- **`Push(key K, value V) (evicted bool)`**  
  Inserts a key-value pair. Returns `true` if an eviction occurred.

- **`Load(key K) (V, bool)`**  
  Retrieves a value for the key.

- **`Has(key K) bool`**  
  Checks if a key exists in the cache.

- **`Delete(key K) bool`**  
  Removes a key. Returns `true` if the key existed. The eviction callback is invoked if present.

- **`Clear()`**  
  Removes all entries from the cache. The eviction callback is invoked for each item.

- **`Size() int`**  
  Returns the current number of items.

- **`Capacity() int`**  
  Returns the maximum capacity.

# Test
```shell
go test -coverpkg=github.com/chi07/rediscache -cover ./...
```