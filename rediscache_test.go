package rediscache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/chi07/rediscache"
)

// newTestCache tạo Cache + miniredis (in-memory)
func newTestCache(t *testing.T) (*rediscache.Cache, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	c := rediscache.New(rdb, rediscache.Options{
		TTL:             2 * time.Minute,
		KeyPrefix:       "test",
		ReadTimeout:     300 * time.Millisecond,
		WriteTimeout:    300 * time.Millisecond,
		PipelineTimeout: 800 * time.Millisecond,
	})
	return c, mr
}

func TestKeyAndNormalize(t *testing.T) {
	c, _ := newTestCache(t)

	if got, want := c.Key("course", "by_id"), "test:course:by_id"; got != want {
		t.Fatalf("Key() = %q; want %q", got, want)
	}

	cases := map[string]string{
		"  Hello   WORLD  ": "hello world",
		"\tGo\tLang\n":      "go lang",
		"":                  "",
		"   ":               "",
		"Đ ấ y  ":           "đ ấ y",
	}
	for in, want := range cases {
		if got := rediscache.Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestAtomicReplaceHash(t *testing.T) {
	ctx := context.Background()
	c, mr := newTestCache(t)

	key := c.Key("group", "name2id")

	// Lần 1: set bản đầy đủ
	m := map[string]string{"backend": "9", "mobile": "3"}
	if err := c.AtomicReplaceHash(ctx, key, m); err != nil {
		t.Fatalf("AtomicReplaceHash error: %v", err)
	}

	// Kiểm tra HGETALL
	h, err := c.RDB.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatalf("HGetAll error: %v", err)
	}
	if len(h) != 2 || h["backend"] != "9" || h["mobile"] != "3" {
		t.Fatalf("unexpected hash content: %+v", h)
	}

	// TTL > 0 và <= TTL config
	ttl, err := c.RDB.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL error: %v", err)
	}
	if ttl <= 0 || ttl > c.Opts.TTL {
		t.Fatalf("unexpected TTL: %v", ttl)
	}

	// Lần 2: thay thế bằng map khác (đảm bảo atomic)
	m2 := map[string]string{"backend": "9", "frontend": "1"}
	if err := c.AtomicReplaceHash(ctx, key, m2); err != nil {
		t.Fatalf("AtomicReplaceHash2 error: %v", err)
	}
	h2, err := c.RDB.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatalf("HGetAll2 error: %v", err)
	}
	if len(h2) != 2 || h2["backend"] != "9" || h2["frontend"] != "1" {
		t.Fatalf("unexpected hash content after replace: %+v", h2)
	}

	// Không còn tmp key rò rỉ
	for _, k := range mr.Keys() { // Keys() => map[int]string ; range lấy value là string
		if strings.Contains(k, ":tmp:") {
			t.Fatalf("tmp key leaked: %s", k)
		}
	}
}

func TestAtomicReplaceHash_EmptyMapCreatesKey(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("empty", "hash")

	// empty map vẫn tạo hash rỗng (sau rename)
	if err := c.AtomicReplaceHash(ctx, key, map[string]string{}); err != nil {
		t.Fatalf("AtomicReplaceHash(empty) error: %v", err)
	}

	// Đọc HLen = 0 (hash tồn tại và rỗng)
	n, err := c.RDB.HLen(ctx, key).Result()
	if err != nil {
		t.Fatalf("HLen error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected HLen=0; got %d", n)
	}
}

func TestAtomicReplaceHashJSON_AndHGetJSON(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	type Group struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	key := c.Key("group", "by_id")
	src := map[string]any{
		"9": Group{ID: 9, Name: "Backend"},
		"3": Group{ID: 3, Name: "Mobile"},
	}
	if err := c.AtomicReplaceHashJSON(ctx, key, src); err != nil {
		t.Fatalf("AtomicReplaceHashJSON error: %v", err)
	}

	// đọc lại từng phần tử bằng generic HGetJSON
	g9, ok, err := rediscache.HGetJSON[Group](ctx, c, key, "9")
	if err != nil || !ok {
		t.Fatalf("HGetJSON 9 error=%v ok=%v", err, ok)
	}
	if g9.ID != 9 || g9.Name != "Backend" {
		t.Fatalf("HGetJSON 9 unexpected: %+v", g9)
	}

	// trường không tồn tại
	_, ok, err = rediscache.HGetJSON[Group](ctx, c, key, "404")
	if err != nil {
		t.Fatalf("HGetJSON 404 err should be nil; got %v", err)
	}
	if ok {
		t.Fatalf("HGetJSON 404: expected ok=false; got ok=true")
	}
}

func TestSetSnapshot_AndTryGetSnapshot(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	type Snapshot struct {
		Data []int `json:"data"`
	}
	key := c.Key("course", "snapshot")

	want := Snapshot{Data: []int{1, 2, 3}}
	if err := c.SetSnapshot(ctx, key, want); err != nil {
		t.Fatalf("SetSnapshot error: %v", err)
	}

	got, ok, err := rediscache.TryGetSnapshot[Snapshot](ctx, c, key)
	if err != nil || !ok {
		t.Fatalf("TryGetSnapshot error=%v ok=%v", err, ok)
	}
	if len(got.Data) != 3 || got.Data[0] != 1 || got.Data[2] != 3 {
		t.Fatalf("TryGetSnapshot unexpected: %+v", got)
	}

	// key không tồn tại
	_, ok, err = rediscache.TryGetSnapshot[Snapshot](ctx, c, c.Key("missing"))
	if err != nil {
		t.Fatalf("TryGetSnapshot missing err should be nil; got %v", err)
	}
	if ok {
		t.Fatalf("TryGetSnapshot missing ok should be false")
	}
}

func TestHGetString(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("course", "name2id")

	// set trước
	if err := c.RDB.HSet(ctx, key, "golang", "35").Err(); err != nil {
		t.Fatalf("HSet prep error: %v", err)
	}
	// đọc ok
	v, ok, err := c.HGetString(ctx, key, "golang")
	if err != nil || !ok || v != "35" {
		t.Fatalf("HGetString got v=%q ok=%v err=%v", v, ok, err)
	}

	// field không tồn tại
	_, ok, err = c.HGetString(ctx, key, "missing")
	if err != nil {
		t.Fatalf("HGetString missing err should be nil; got %v", err)
	}
	if ok {
		t.Fatalf("HGetString missing ok should be false")
	}
}

func TestAtomicReplaceHashJSON_EmptyMapCreatesKey(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("empty", "hashjson")

	// empty map vẫn tạo hash rỗng (sau rename)
	if err := c.AtomicReplaceHashJSON(ctx, key, map[string]any{}); err != nil {
		t.Fatalf("AtomicReplaceHashJSON(empty) error: %v", err)
	}

	// Đọc HLen = 0 (hash tồn tại và rỗng)
	n, err := c.RDB.HLen(ctx, key).Result()
	if err != nil {
		t.Fatalf("HLen error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected HLen=0; got %d", n)
	}
}

// ---- Các test unmarshal error: dùng JSON không hợp lệ ----

func TestTryGetSnapshot_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("snap", "bad")

	// Ghi JSON không hợp lệ (không dùng SetSnapshot vì nó luôn marshal hợp lệ)
	if err := c.RDB.Set(ctx, key, "not-json", c.Opts.TTL).Err(); err != nil {
		t.Fatalf("prep Set invalid JSON error: %v", err)
	}

	type Any struct{ X int } // kiểu bất kỳ
	_, ok, err := rediscache.TryGetSnapshot[Any](ctx, c, key)
	if ok {
		t.Fatalf("expected ok=false due to unmarshal error")
	}
	if err == nil {
		t.Fatalf("expected unmarshal error; got nil")
	}
}

func TestHGetJSON_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("hash", "badjson")

	// Ghi JSON không hợp lệ vào field
	if err := c.RDB.HSet(ctx, key, "field", "not-json").Err(); err != nil {
		t.Fatalf("prep HSet invalid JSON error: %v", err)
	}

	type Y struct{ B string }
	_, ok, err := rediscache.HGetJSON[Y](ctx, c, key, "field")
	if ok {
		t.Fatalf("expected ok=false due to unmarshal error")
	}
	if err == nil {
		t.Fatalf("expected unmarshal error; got nil")
	}
}

func TestHGetString_RedisNilPath(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("hash", "nilpath")

	// không set gì → redis.Nil path
	_, ok, err := c.HGetString(ctx, key, "nope")
	if err != nil {
		t.Fatalf("HGetString err should be nil on redis.Nil path; got %v", err)
	}
	if ok {
		t.Fatalf("ok should be false on redis.Nil path")
	}
}

func TestTryGetSnapshot_RedisNilPath(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	// key chưa tồn tại
	_, ok, err := rediscache.TryGetSnapshot[struct{ X int }](ctx, c, c.Key("no", "snap"))
	if err != nil {
		t.Fatalf("TryGetSnapshot err should be nil on redis.Nil path; got %v", err)
	}
	if ok {
		t.Fatalf("ok should be false on redis.Nil path")
	}
}

func TestHGetJSON_RedisNilPath(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestCache(t)

	key := c.Key("hash", "jsonnil")
	type T struct{ N int }

	_, ok, err := rediscache.HGetJSON[T](ctx, c, key, "notfound")
	if err != nil {
		t.Fatalf("HGetJSON err should be nil on redis.Nil path; got %v", err)
	}
	if ok {
		t.Fatalf("ok should be false on redis.Nil path")
	}
}
