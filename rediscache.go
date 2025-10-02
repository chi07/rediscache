package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Options struct {
	TTL             time.Duration
	KeyPrefix       string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PipelineTimeout time.Duration
}

func (o Options) withDefaults() Options {
	if o.TTL <= 0 {
		o.TTL = 10 * time.Minute
	}
	if o.KeyPrefix == "" {
		o.KeyPrefix = "app"
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = 300 * time.Millisecond
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 500 * time.Millisecond
	}
	if o.PipelineTimeout <= 0 {
		o.PipelineTimeout = 1 * time.Second
	}
	return o
}

type Cache struct {
	RDB  *redis.Client
	Opts Options
}

func New(rdb *redis.Client, opts Options) *Cache {
	return &Cache{RDB: rdb, Opts: opts.withDefaults()}
}

func (c *Cache) Key(parts ...string) string {
	all := make([]string, 0, 1+len(parts))
	all = append(all, c.Opts.KeyPrefix)
	all = append(all, parts...)
	return strings.Join(all, ":")
}

func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	sp := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !sp {
				b.WriteByte(' ')
				sp = true
			}
		} else {
			b.WriteRune(r)
			sp = false
		}
	}
	return b.String()
}

// ---------- Atomic writers (methods, NON-generic) ----------

func (c *Cache) AtomicReplaceHash(ctx context.Context, finalKey string, kv map[string]string) error {
	rc, cancel := context.WithTimeout(ctx, c.Opts.PipelineTimeout)
	defer cancel()

	tmpKey := finalKey + ":tmp:" + uuid.NewString()
	pipe := c.RDB.Pipeline()

	// 1) Tạo tmpKey
	dummy := false
	if len(kv) > 0 {
		args := make([]any, 0, len(kv)*2)
		for k, v := range kv {
			args = append(args, k, v)
		}
		pipe.HSet(rc, tmpKey, args...)
	} else {
		// Đảm bảo tmpKey tồn tại để RENAME không lỗi
		pipe.HSet(rc, tmpKey, "___", "___")
		dummy = true
	}

	// 2) Đổi tên sang finalKey + TTL
	pipe.Rename(rc, tmpKey, finalKey)
	pipe.Expire(rc, finalKey, c.Opts.TTL)

	// 3) Nếu dùng dummy, xóa field dummy TRÊN finalKey sau khi rename
	if dummy {
		pipe.HDel(rc, finalKey, "___")
	}

	_, err := pipe.Exec(rc)
	return err
}

func (c *Cache) AtomicReplaceHashJSON(ctx context.Context, finalKey string, objs map[string]any) error {
	rc, cancel := context.WithTimeout(ctx, c.Opts.PipelineTimeout)
	defer cancel()

	tmpKey := finalKey + ":tmp:" + uuid.NewString()
	pipe := c.RDB.Pipeline()

	dummy := false
	if len(objs) > 0 {
		for id, obj := range objs {
			b, _ := json.Marshal(obj) // best-effort
			pipe.HSet(rc, tmpKey, id, b)
		}
	} else {
		pipe.HSet(rc, tmpKey, "___", "___")
		dummy = true
	}

	pipe.Rename(rc, tmpKey, finalKey)
	pipe.Expire(rc, finalKey, c.Opts.TTL)

	if dummy {
		pipe.HDel(rc, finalKey, "___")
	}

	_, err := pipe.Exec(rc)
	return err
}

func (c *Cache) SetSnapshot(ctx context.Context, key string, snapshot any) error {
	rc, cancel := context.WithTimeout(ctx, c.Opts.WriteTimeout)
	defer cancel()

	b, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return c.RDB.Set(rc, key, b, c.Opts.TTL).Err()
}

// ---------- Generic FUNCTIONS ----------

// TryGetSnapshot: GET key rồi unmarshal ra T

func TryGetSnapshot[T any](ctx context.Context, c *Cache, key string) (T, bool, error) {
	var zero T

	rc, cancel := context.WithTimeout(ctx, c.Opts.ReadTimeout)
	defer cancel()

	raw, err := c.RDB.Get(rc, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}

	var out T
	if uErr := json.Unmarshal(raw, &out); uErr != nil {
		return zero, false, uErr
	}
	return out, true, nil
}

// HGetJSON: HGET field rồi unmarshal ra T

func HGetJSON[T any](ctx context.Context, c *Cache, key, field string) (T, bool, error) {
	var zero T

	rc, cancel := context.WithTimeout(ctx, c.Opts.ReadTimeout)
	defer cancel()

	raw, err := c.RDB.HGet(rc, key, field).Bytes()
	if errors.Is(err, redis.Nil) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}

	var out T
	if uErr := json.Unmarshal(raw, &out); uErr != nil {
		return zero, false, uErr
	}
	return out, true, nil
}

// HGetString: HGET field trả về string (non-generic)

func (c *Cache) HGetString(ctx context.Context, key, field string) (string, bool, error) {
	rc, cancel := context.WithTimeout(ctx, c.Opts.ReadTimeout)
	defer cancel()

	v, err := c.RDB.HGet(rc, key, field).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
