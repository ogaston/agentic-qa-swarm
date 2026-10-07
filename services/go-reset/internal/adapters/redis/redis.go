// Package redis es el CacheFlusher sobre RESP (FLUSHALL / DBSIZE), sin dependencias.
package redis

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Flusher struct {
	Addr    string
	Timeout time.Duration
}

func (f *Flusher) do(ctx context.Context, args ...string) (string, error) {
	t := f.Timeout
	if t == 0 {
		t = 5 * time.Second
	}
	d := net.Dialer{Timeout: t}
	conn, err := d.DialContext(ctx, "tcp", f.Addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(t))
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(sb.String())); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(line, "-") {
		return "", fmt.Errorf("redis: %s", line[1:])
	}
	return line, nil
}

func (f *Flusher) Flush(ctx context.Context) error {
	r, err := f.do(ctx, "FLUSHALL")
	if err != nil {
		return err
	}
	if r != "+OK" {
		return fmt.Errorf("FLUSHALL: respuesta %q", r)
	}
	return nil
}

func (f *Flusher) DBSize(ctx context.Context) (int64, error) {
	r, err := f.do(ctx, "DBSIZE")
	if err != nil {
		return 0, err
	}
	if !strings.HasPrefix(r, ":") {
		return 0, fmt.Errorf("DBSIZE: respuesta %q", r)
	}
	return strconv.ParseInt(r[1:], 10, 64)
}
