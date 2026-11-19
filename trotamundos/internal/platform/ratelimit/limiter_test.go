package ratelimit

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"trotamundos/internal/platform/redisx"
)

func TestMemoryLimiterFixedWindow(t *testing.T) {
	m := NewMemoryLimiter(2, time.Minute)
	base := time.Now()
	m.now = func() time.Time { return base }
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		ok, err := m.Allow(ctx, "ip")
		if err != nil || !ok {
			t.Fatalf("request %d debió ser permitido: %v, %v", i, ok, err)
		}
	}
	if ok, err := m.Allow(ctx, "ip"); err != nil || ok {
		t.Fatalf("el tercer request debía ser rechazado: %v, %v", ok, err)
	}
	if ok, err := m.Allow(ctx, "otra-ip"); err != nil || !ok {
		t.Fatalf("otra clave no debía estar limitada: %v, %v", ok, err)
	}

	m.now = func() time.Time { return base.Add(time.Minute + time.Second) }
	if ok, err := m.Allow(ctx, "ip"); err != nil || !ok {
		t.Fatalf("la ventana nueva debía permitir el request: %v, %v", ok, err)
	}
}

func TestInvalidConfigFailsClosed(t *testing.T) {
	m := NewMemoryLimiter(0, time.Minute)
	if ok, _ := m.Allow(context.Background(), "ip"); ok {
		t.Fatal("un limitador sin límite no debe permitir requests")
	}
	l := NewRedisLimiter(nil, 1, time.Minute)
	if ok, err := l.Allow(context.Background(), "ip"); ok || err == nil {
		t.Fatalf("un RedisLimiter sin cliente debe fallar: %v, %v", ok, err)
	}
}

func TestRedisLimiterUsesAtomicEval(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var mu sync.Mutex
	var commands [][]string
	counts := map[string]int{}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		for {
			args, err := readRESPCommand(r)
			if err != nil {
				return
			}
			mu.Lock()
			commands = append(commands, args)
			mu.Unlock()
			if len(args) > 4 && args[0] == "EVAL" {
				key := args[4]
				counts[key]++
				_, _ = fmt.Fprintf(conn, ":%d\r\n", counts[key])
			} else {
				_, _ = fmt.Fprint(conn, "+OK\r\n")
			}
		}
	}()

	client := redisx.New(ln.Addr().String(), "", 0)
	defer client.Close()
	limiter := NewRedisLimiter(client, 2, time.Minute)

	for i, want := range []bool{true, true, false} {
		got, err := limiter.Allow(context.Background(), "ip")
		if err != nil {
			t.Fatalf("Allow %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("Allow %d = %v, quería %v", i, got, want)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 3 {
		t.Fatalf("esperaba 3 comandos, obtuve %d", len(commands))
	}
	args := commands[0]
	if args[0] != "EVAL" || args[1] != incrAndExpireScript || args[2] != "1" {
		t.Fatalf("comando inesperado: %q", args)
	}
	if !strings.Contains(args[1], "INCR") || !strings.Contains(args[1], "PEXPIRE") {
		t.Fatalf("el script debe hacer INCR y PEXPIRE: %q", args[1])
	}
	if args[3] != "ratelimit:ip" || args[4] != "2" || args[5] != "60000" {
		t.Fatalf("argumentos inesperados: %q", args)
	}
	if limiter.Window() != time.Minute {
		t.Fatalf("Window = %v", limiter.Window())
	}
}

func readRESPCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}
