package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Snapbug/gomemcache/memcache"
)

func TestAcceptance(t *testing.T) {
	done := false

	addr := "localhost:11211"
	// MEMCACHED_PORT might be set by a linked memcached docker container.
	if env := os.Getenv("MEMCACHED_PORT"); env != "" {
		addr = strings.TrimLeft(env, "tcp://")
	}

	exporter := exec.Command("./memcached_exporter", "-memcached.address", addr)
	go func() {
		if err := exporter.Run(); err != nil && !done {
			t.Fatal(err)
		}
	}()
	defer func() {
		if exporter.Process != nil {
			exporter.Process.Kill()
		}
	}()

	defer func() {
		done = true
	}()

	// TODO(ts): Replace sleep with ready check loop.
	time.Sleep(100 * time.Millisecond)

	client := memcache.New(addr)
	// TODO(ts): reset stats first.
	item := &memcache.Item{Key: "foo", Value: []byte("bar")}
	if err := client.Set(item); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(item); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get("foo"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get("qux"); err != memcache.ErrCacheMiss {
		t.Fatal(err)
	}
	last, err := client.Get("foo")
	if err != nil {
		t.Fatal(err)
	}
	last.Value = []byte("banana")
	if err := client.CompareAndSwap(last); err != nil {
		t.Fatal(err)
	}
	large := &memcache.Item{Key: "large", Value: []byte("Hello World this text should be 128 bytes in size so that we may test the slab functionality of memcached. Filler text here to  ")}
	if err := client.Set(large); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get("http://localhost:9150/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{
		`memcached_up 1`,
		`memcached_commands_total{command="get",status="hit"} 2`,
		`memcached_commands_total{command="get",status="miss"} 1`,
		`memcached_commands_total{command="set",status="hit"} 3`,
		`memcached_commands_total{command="cas",status="hit"} 1`,
		`memcached_current_bytes 274`,
		`memcached_current_connections 11`,
		`memcached_current_items 2`,
		`memcached_items_total 4`,
		`memcached_active_slabs_total 2`,
		`memcached_items_number_total{slab="1"} 1`,
		`memcached_slabs_commands_total{command="set",slab="1",status="hit"} 2`,
		`memcached_slabs_commands_total{command="cas",slab="1",status="hit"} 1`,
	}
	for _, test := range tests {
		if !bytes.Contains(body, []byte(test)) {
			t.Errorf("want metrics to include %q, have:\n%s", test, body)
		}
	}
}
