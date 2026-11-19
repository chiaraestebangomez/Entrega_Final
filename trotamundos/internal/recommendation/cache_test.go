package recommendation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/redisx"
)

// miniRedis implementa el mínimo protocolo RESP necesario para verificar que
// redisx.Keys recorre SCAN con MATCH (devuelve una clave por cursor) y que
// RedisCache borra por prefijo/patrón sin usar KEYS.
type miniRedis struct {
	mu   sync.Mutex
	keys map[string]bool
}

func startMiniRedis(t *testing.T, initial ...string) (*redisx.Client, *miniRedis) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &miniRedis{keys: map[string]bool{}}
	for _, key := range initial {
		server.keys[key] = true
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.serve(conn)
		}
	}()
	client := redisx.New(listener.Addr().String(), "", 0)
	t.Cleanup(func() {
		_ = client.Close()
		_ = listener.Close()
	})
	return client, server
}

func (m *miniRedis) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "PING":
			_, _ = io.WriteString(conn, "+PONG\r\n")
		case "SET":
			m.mu.Lock()
			m.keys[args[1]] = true
			m.mu.Unlock()
			_, _ = io.WriteString(conn, "+OK\r\n")
		case "GET":
			m.mu.Lock()
			_, ok := m.keys[args[1]]
			m.mu.Unlock()
			if ok {
				writeBulk(conn, "value")
			} else {
				_, _ = io.WriteString(conn, "$-1\r\n")
			}
		case "DEL":
			m.mu.Lock()
			deleted := 0
			for _, key := range args[1:] {
				if m.keys[key] {
					delete(m.keys, key)
					deleted++
				}
			}
			m.mu.Unlock()
			_, _ = fmt.Fprintf(conn, ":%d\r\n", deleted)
		case "SCAN":
			m.scan(conn, args)
		default:
			_, _ = io.WriteString(conn, "-ERR unsupported command\r\n")
		}
	}
}

func (m *miniRedis) scan(conn io.Writer, args []string) {
	cursor, _ := strconv.Atoi(args[1])
	pattern := ""
	for i := 2; i+1 < len(args); i++ {
		if strings.EqualFold(args[i], "MATCH") {
			pattern = args[i+1]
		}
	}
	m.mu.Lock()
	all := make([]string, 0, len(m.keys))
	for key := range m.keys {
		all = append(all, key)
	}
	m.mu.Unlock()
	sort.Strings(all)

	matching := []string{}
	for _, key := range all {
		matched, err := path.Match(pattern, key)
		if err == nil && matched {
			matching = append(matching, key)
		}
	}
	_, _ = io.WriteString(conn, "*2\r\n")
	next := "0"
	batch := []string{}
	if cursor >= 0 && cursor < len(matching) {
		batch = append(batch, matching[cursor])
		if cursor+1 < len(matching) {
			next = strconv.Itoa(cursor + 1)
		}
	}
	writeBulk(conn, next)
	writeArray(conn, batch)
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if line == "" || line[0] != '*' {
		return nil, fmt.Errorf("resp: se esperaba array, got %q", line)
	}
	count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, count)
	for i := 0; i < count; i++ {
		line, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "" || line[0] != '$' {
			return nil, fmt.Errorf("resp: se esperaba bulk string, got %q", line)
		}
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil || size < 0 {
			return nil, fmt.Errorf("resp: tamaño inválido %q", line)
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:size])
	}
	return args, nil
}

func writeBulk(w io.Writer, value string) {
	_, _ = fmt.Fprintf(w, "$%d\r\n%s\r\n", len(value), value)
}

func writeArray(w io.Writer, values []string) {
	_, _ = fmt.Fprintf(w, "*%d\r\n", len(values))
	for _, value := range values {
		writeBulk(w, value)
	}
}

func TestMemoryCacheDeleteByPrefixAndPattern(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCache()
	for _, key := range []string{resultPrefix + "v1:user-1", resultPrefix + "user-1", resultPrefix + "v1:user-2", snapshotKey} {
		if err := cache.Set(ctx, key, "value", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := cache.DeletePattern(ctx, resultPrefix+"*"+escapeGlob("user-1"))
	if err != nil || len(deleted) != 2 {
		t.Fatalf("deleted = %+v, err = %v", deleted, err)
	}
	if err := cache.DeleteByPrefix(ctx, resultPrefix); err != nil {
		t.Fatal(err)
	}
	keys, err := cache.Keys(ctx, "*")
	if err != nil || len(keys) != 1 || keys[0] != snapshotKey {
		t.Fatalf("keys = %+v, err = %v", keys, err)
	}
}

func TestRedisKeysUsesSCANMatchAndFollowsCursor(t *testing.T) {
	client, _ := startMiniRedis(t,
		resultPrefix+"v1:user-1", resultPrefix+"v2:user-1", resultPrefix+"v1:user-2", snapshotKey)
	keys, err := client.Keys(context.Background(), resultPrefix+"*"+escapeGlob("user-1"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{resultPrefix + "v1:user-1", resultPrefix + "v2:user-1"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %+v, want %+v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %+v, want %+v", keys, want)
		}
	}
}

func TestRedisCacheDeletePatternAndPrefix(t *testing.T) {
	client, server := startMiniRedis(t,
		resultPrefix+"v1:user-1", resultPrefix+"user-1", resultPrefix+"v1:user-2", snapshotKey)
	cache := NewRedisCache(client)

	deleted, err := cache.DeletePattern(context.Background(), resultPrefix+"*"+escapeGlob("user-1"))
	if err != nil || len(deleted) != 2 {
		t.Fatalf("deleted = %+v, err = %v", deleted, err)
	}
	if err := cache.DeleteByPrefix(context.Background(), resultPrefix); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.keys) != 1 || !server.keys[snapshotKey] {
		t.Fatalf("claves restantes = %+v", server.keys)
	}
}

func TestInvalidateWithRedisCacheRemovesAllVersions(t *testing.T) {
	ctx := context.Background()
	client, server := startMiniRedis(t,
		resultPrefix+"v0:user-1", resultPrefix+"user-1", resultPrefix+"v0:user-2")
	cache := NewRedisCache(client)
	if err := cache.Set(ctx, snapshotKey, domain.ModelSnapshot{Version: "v9"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := cache.Set(ctx, resultPrefix+"v9:user-1", []domain.Recommendation{}, time.Minute); err != nil {
		t.Fatal(err)
	}
	svc := testService(t, newFakeStore(), cache, newFakeAgencies(), &fakeCatalog{})

	if _, err := svc.UpdatePreferences(ctx, "user-1", domain.RecommendationPreferences{}); err != nil {
		t.Fatalf("update: %v", err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	shouldBeGone := []string{
		resultPrefix + "v0:user-1",
		resultPrefix + "user-1",
		resultPrefix + "v9:user-1",
	}
	for _, key := range shouldBeGone {
		if server.keys[key] {
			t.Fatalf("clave %q sigue en caché", key)
		}
	}
	if !server.keys[resultPrefix+"v0:user-2"] {
		t.Fatal("se borró la caché de otro usuario")
	}
}
