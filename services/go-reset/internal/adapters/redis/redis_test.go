package redis_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/adapters/redis"
)

// fakeRedis atiende FLUSHALL y DBSIZE con un contador de claves real.
func fakeRedis(t *testing.T, keys *int, reply map[string]string) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				var parts []string
				first, _ := r.ReadString('\n')
				n := 0
				for _, ch := range strings.TrimSpace(first)[1:] {
					n = n*10 + int(ch-'0')
				}
				for i := 0; i < n; i++ {
					_, _ = r.ReadString('\n')
					v, _ := r.ReadString('\n')
					parts = append(parts, strings.TrimSpace(v))
				}
				switch parts[0] {
				case "FLUSHALL":
					*keys = 0
					c.Write([]byte(orDefault(reply["FLUSHALL"], "+OK\r\n")))
				case "DBSIZE":
					c.Write([]byte(orDefault(reply["DBSIZE"], ":"+itoa(*keys)+"\r\n")))
				}
			}()
		}
	}()
	return l.Addr().String()
}

func orDefault(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func itoa(n int) string {
	return strings.TrimSpace(strings.Join([]string{string(rune('0' + n%10))}, ""))
}

func TestRedisFlushAndDBSizeReadBack(t *testing.T) {
	keys := 7
	f := &redis.Flusher{Timeout: 5 * time.Second, Addr: fakeRedis(t, &keys, map[string]string{"DBSIZE": ":7\r\n"})}
	if n, err := f.DBSize(context.Background()); err != nil || n != 7 {
		t.Fatalf("%d %v", n, err)
	}
	if err := f.Flush(context.Background()); err != nil || keys != 0 {
		t.Fatalf("%v keys=%d", err, keys)
	}
}

func TestRedisErrorReplies(t *testing.T) {
	k := 0
	f := &redis.Flusher{Timeout: 5 * time.Second, Addr: fakeRedis(t, &k, map[string]string{"FLUSHALL": "-ERR denied\r\n", "DBSIZE": "$3\r\n"})}
	if err := f.Flush(context.Background()); err == nil {
		t.Fatal("error de redis ignorado")
	}
	if _, err := f.DBSize(context.Background()); err == nil {
		t.Fatal("respuesta inválida aceptada")
	}
}

func TestRedisUnreachable(t *testing.T) {
	f := &redis.Flusher{Addr: "127.0.0.1:1", Timeout: time.Second}
	if err := f.Flush(context.Background()); err == nil {
		t.Fatal("sin servidor debe fallar")
	}
}
