# rediscache

[![Go Report Card](https://goreportcard.com/badge/github.com/chi07/rediscache)](https://goreportcard.com/report/github.com/chi07/rediscache) |
[![codecov](https://codecov.io/gh/chi07/rediscache/branch/main/graph/badge.svg)](https://codecov.io/gh/chi07/rediscache) |
[![CI](https://github.com/chi07/rediscache/actions/workflows/ci.yml/badge.svg)](https://github.com/chi07/rediscache/actions/workflows/ci.yml)

Nhẹ, nhanh, và an toàn để dựng cache tầng Redis với:
- **Atomic refresh** bằng `tmp → RENAME` (không lộ trạng thái dở dang)
- **Snapshot** (String) & **Hash** (by-id, index)
- Tiện **generic helpers**: `TryGetSnapshot[T]`, `HGetJSON[T]`
- Timeout riêng cho **read/write/pipeline**
- Key prefix tiện cho multi-tenant/microservice

> Go 1.21+ · go-redis/v9

## Installation

```bash
go get github.com/chi07/rediscache
```
Usage

1. Khởi tạo
```go
import (
"github.com/redis/go-redis/v9"
cache "github.com/chi07/rediscache"
"time"
)

rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
c := cache.New(rdb, cache.Options{
TTL:             15 * time.Minute,
KeyPrefix:       "svc",
ReadTimeout:     300 * time.Millisecond,
WriteTimeout:    600 * time.Millisecond,
PipelineTimeout: 1 * time.Second,
})
```

2. Keys & Normalize
```go
k := c.Key("course", "by_id") // "svc:course:by_id"
n := cache.Normalize("  Hello   World ") // "hello world"
```

3. Atomic replace hash

```go
err := c.AtomicReplaceHash(ctx, c.Key("group", "name2id"), map[string]string{
  "backend": "9",
  "mobile":  "3",
})
```

Atomic replace hash (giá trị JSON)
```go
type Group struct{ ID int; Name string }

byID := map[string]any{
  "9": Group{ID: 9, Name: "Backend"},
  "3": Group{ID: 3, Name: "Mobile"},
}
_ = c.AtomicReplaceHashJSON(ctx, c.Key("group", "by_id"), byID)
```

Snapshot (String JSON)
```go
snap := struct {
  Data []int `json:"data"`
}{Data: []int{1,2,3}}

_ = c.SetSnapshot(ctx, c.Key("course", "snapshot"), snap)

// Đọc lại (generic)
var got struct{ Data []int `json:"data"` }
if v, ok, err := cache.TryGetSnapshot[struct{ Data []int `json:"data"` }](ctx, c, c.Key("course", "snapshot")); err == nil && ok {
  got = v
}
```

HGET JSON (generic) & String
```go
// JSON
type Course struct{ ID int; Name string }
v, ok, err := cache.HGetJSON[Course](ctx, c, c.Key("course", "by_id"), "35")

// String
s, ok, err := c.HGetString(ctx, c.Key("course", "name2id"), "golang")
```

Best practices
•	Dùng snapshot cho list (String JSON), hash cho by_id và name2id.
•	Mọi rebuild lớn dùng AtomicReplace* để switch key an toàn.
•	Khi miss cache: refresh rồi thử lại, nếu vẫn miss → fallback DB/API.
•	Kết hợp upsert theo sự kiện + full sweep định kỳ để chống lệch dữ liệu.

# Test
```shell
go test -coverpkg=github.com/chi07/rediscache -cover ./...
```
