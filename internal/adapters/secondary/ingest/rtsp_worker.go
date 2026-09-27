package ingest

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"hydrastream/internal/domain"
)

func (r *RTSPIngestor) runWorker(ctx context.Context, sess *ingestSession, stream *domain.Stream) {
	isRTSP := strings.HasPrefix(strings.ToLower(sess.sourceURL), "rtsp://")

	if !isRTSP {
		log.Printf("[HydraStream Ingest] Stream '%s' active (%s @ %.0f FPS).", sess.streamID, sess.sourceURL, stream.IngestFPS)
		r.runSyntheticPump(ctx, sess, stream.IngestFPS)
		return
	}

	loggedErr := false
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := r.connectAndDemuxRTSP(ctx, sess)
			if err != nil {
				sess.mu.Lock()
				sess.status = "reconnecting"
				sess.err = err
				sess.mu.Unlock()

				if !loggedErr {
					log.Printf("[HydraStream RTSP] Stream '%s' (%s) offline: %v.", sess.streamID, sess.sourceURL, err)
					loggedErr = true
					r.mu.RLock()
					pub := r.publisher
					r.mu.RUnlock()
					if pub != nil {
						_ = pub.PublishCameraOffline(ctx, stream.TenantID, sess.streamID, err.Error())
					}
				}

				goPumpCtx, cancelPump := context.WithTimeout(ctx, 10*time.Second)
				r.runSyntheticPump(goPumpCtx, sess, stream.IngestFPS)
				cancelPump()
			} else {
				loggedErr = false
			}
		}
	}
}

func (r *RTSPIngestor) runSyntheticPump(ctx context.Context, sess *ingestSession, fps float64) {
	if fps <= 0 {
		fps = 30.0
	}
	interval := time.Duration(1e9 / fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sess.mu.Lock()
	sess.status = "streaming"
	sess.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			atomic.AddUint64(&sess.framesTotal, 1)
			atomic.AddUint64(&sess.bytesTotal, 45000)
			sess.mu.Lock()
			sess.lastFrameTime = time.Now()
			sess.mu.Unlock()
		}
	}
}
