package skyapi

import (
	"fmt"
	"io"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"
)

var concurrency = 16

func init() {
	go http.ListenAndServe("localhost:6060", nil)
}

func BenchmarkMultiplexer(b *testing.B) {
	var limit = int64(b.N * 1024)

	var multiplexer = SkyNet.WithLoopBack()

	type cr struct {
		nRead int64
		rErr  error
		rnd   *RndReader
	}

	var r = make(chan cr, concurrency)

	for i := 0; i < concurrency; i++ {
		var rndA = &RndReader{Limit: int(limit)}
		var srv, srvErr = multiplexer.Bind("", fmt.Sprintf(":%d", 10000+i))
		if srvErr != nil {
			b.Fatal("bind", srvErr)
		}

		go func() {
			for {
				var stream, acceptErr = srv.Accept()
				if acceptErr != nil {
					b.Fatal("Accept", acceptErr)
				}
				go func() {
					io.Copy(stream, rndA)
					stream.Close()
				}()
			}
		}()

		var stream, streamErr = multiplexer.DialTimeout(
			srv.Addr().Network(), srv.Addr().String(),
			time.Second,
		)
		if streamErr != nil {
			b.Fatal(streamErr)
		}

		go func() {
			var nRead, rErr = io.Copy(rndA, stream)
			r <- cr{nRead, rErr, rndA}
		}()
	}

	for i := 0; i < concurrency; i++ {
		var cr = <-r
		var rnd, nRead, rErr = cr.rnd, cr.nRead, cr.rErr

		var rHash = rnd.RChecksum()
		var wHash = rnd.WChecksum()

		if rErr != nil || rHash != wHash {
			b.Error(nRead, limit, rErr, rHash, wHash)
		}
	}

	b.ReportAllocs()
	b.SetBytes(1024 * 2 * int64(concurrency))
}
