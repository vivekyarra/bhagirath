// Command loadtest fires concurrent signed WhatsApp webhooks and page reads at
// a running Bhagirath (started with WHATSAPP_APP_SECRET=loadsecret) and
// reports latency percentiles and errors.
//
//	go run ./tools/loadtest http://localhost:8080
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func main() {
	base := os.Args[1]
	const workers, perWorker = 40, 60
	var mu sync.Mutex
	lat := map[string][]time.Duration{}
	errs := map[string]int{}
	client := &http.Client{Timeout: 30 * time.Second}
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				var req *http.Request
				kind := []string{"webhook", "webhook", "report", "impact", "healthz"}[i%5]
				switch kind {
				case "webhook":
					phone := fmt.Sprintf("9100009%02d%03d", w, i)
					body := fmt.Sprintf(`{"entry":[{"changes":[{"value":{"messages":[{"from":"%s","id":"wamid.L%d.%d","timestamp":"%d","type":"location","location":{"latitude":%f,"longitude":75.8600,"name":"शिव मंदिर"}},{"from":"%s","id":"wamid.T%d.%d","timestamp":"%d","type":"text","text":{"body":"शिव मंदिर के पास नल से बदबूदार गंदा पानी आ रहा है"}}]}}]}]}`,
						phone, w, i, time.Now().Unix(), 22.719+float64(i%10)*0.0002, phone, w, i, time.Now().Unix())
					req, _ = http.NewRequest("POST", base+"/webhooks/whatsapp", strings.NewReader(body))
					m := hmac.New(sha256.New, []byte("loadsecret"))
					m.Write([]byte(body))
					req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
				default:
					req, _ = http.NewRequest("GET", base+map[string]string{"report": "/report", "impact": "/impact", "healthz": "/healthz"}[kind], nil)
				}
				t := time.Now()
				resp, err := client.Do(req)
				d := time.Since(t)
				mu.Lock()
				if err != nil || resp.StatusCode >= 400 {
					code := 0
					if resp != nil {
						code = resp.StatusCode
					}
					errs[fmt.Sprintf("%s:%d", kind, code)]++
				}
				lat[kind] = append(lat[kind], d)
				mu.Unlock()
				if resp != nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(w)
	}
	wg.Wait()
	el := time.Since(start)
	total := workers * perWorker
	fmt.Printf("%d requests in %v (%.0f req/s), errors %v\n", total, el.Round(time.Millisecond), float64(total)/el.Seconds(), errs)
	for k, ds := range lat {
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		p := func(q float64) time.Duration { return ds[int(q*float64(len(ds)-1))].Round(time.Millisecond) }
		fmt.Printf("  %-8s n=%4d p50=%v p95=%v p99=%v max=%v\n", k, len(ds), p(.5), p(.95), p(.99), ds[len(ds)-1].Round(time.Millisecond))
	}
}
