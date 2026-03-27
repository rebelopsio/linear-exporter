package cache

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("key1", "value1")

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %v", val)
	}
}

func TestCache_Expiration(t *testing.T) {
	c := New(10 * time.Millisecond)
	c.Set("key1", "value1")

	time.Sleep(20 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache miss after expiration")
	}
}

func TestCache_SetWithTTL(t *testing.T) {
	c := New(1 * time.Minute)
	c.SetWithTTL("key1", "value1", 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache miss after custom TTL expiration")
	}
}

func TestCache_Clear(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Clear()

	_, ok1 := c.Get("key1")
	_, ok2 := c.Get("key2")
	if ok1 || ok2 {
		t.Fatal("expected all entries cleared")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Invalidate("key1")

	_, ok1 := c.Get("key1")
	_, ok2 := c.Get("key2")
	if ok1 {
		t.Fatal("expected invalidated key to be missing")
	}
	if !ok2 {
		t.Fatal("expected key2 to still exist")
	}
}

func TestCache_MissOnEmpty(t *testing.T) {
	c := New(1 * time.Minute)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss on empty cache")
	}
}

func BenchmarkCache_SetGet(b *testing.B) {
	c := New(1 * time.Minute)
	c.Set("bench", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("bench")
	}
}
