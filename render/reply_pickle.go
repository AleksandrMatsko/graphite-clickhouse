package render

import (
	"bufio"
	"net/http"
	"time"

	"github.com/lomik/graphite-clickhouse/helper/point"
	"github.com/lomik/graphite-clickhouse/helper/rollup"
	"github.com/lomik/graphite-clickhouse/pkg/scope"
	pickle "github.com/lomik/graphite-pickle"
	"go.uber.org/zap"
)

func (h *Handler) ReplyPickle(w http.ResponseWriter, r *http.Request, data *Data, from, until uint32, prefix string, rollupObj *rollup.Rules) {
	var rollupTime time.Duration
	var pickleTime time.Duration

	points := data.Points.List()

	logger := scope.Logger(r.Context())

	defer func() {
		logger.Debug("rollup",
			zap.String("runtime", rollupTime.String()),
			zap.Duration("runtime_ns", rollupTime),
		)
		logger.Debug("pickle",
			zap.String("runtime", pickleTime.String()),
			zap.Duration("runtime_ns", pickleTime),
		)
	}()

	if len(points) == 0 {
		w.Write(pickle.EmptyList)
		return
	}

	writer := bufio.NewWriterSize(w, 1024*1024)
	p := pickle.NewWriter(writer)
	defer writer.Flush()

	p.List()

	writeAlias := func(name string, pathExpression string, points []point.Point, step uint32) {
		pickleStart := time.Now()
		p.Dict()

		p.String("name")
		p.String(name)
		p.SetItem()

		p.String("pathExpression")
		p.String(pathExpression)
		p.SetItem()

		p.String("step")
		p.Uint32(step)
		p.SetItem()

		start := from - (from % step)
		if start < from {
			start += step
		}
		end := until - (until % step)
		last := start - step

		p.String("values")
		p.List()
		for _, point := range points {
			if point.Time < start || point.Time > end {
				continue
			}

			if point.Time > last+step {
				p.AppendNulls(int(((point.Time - last) / step) - 1))
			}

			p.AppendFloat64(point.Value)

			last = point.Time
		}

		if end > last {
			p.AppendNulls(int((end - last) / step))
		}
		p.SetItem()

		p.String("start")
		p.Uint32(uint32(start))
		p.SetItem()

		p.String("end")
		p.Uint32(uint32(end))
		p.SetItem()

		p.Append()
		pickleTime += time.Since(pickleStart)
	}

	writeMetric := func(points []point.Point) {
		metricName := data.Points.MetricName(points[0].MetricID)
		rollupStart := time.Now()
		points, step, err := rollupObj.RollupMetric(metricName, from, points)
		if err != nil {
			logger.Error("rollup failed", zap.Error(err))
			return
		}
		rollupTime += time.Since(rollupStart)

		a := data.Aliases[metricName]
		for k := 0; k < len(a); k += 2 {
			writeAlias(a[k], a[k+1], points, step)
		}
	}
	// group by Metric
	var i, n int
	// i - current position of iterator
	// n - position of the first record with current metric
	l := len(points)

	for i = 1; i < l; i++ {
		if points[i].MetricID != points[n].MetricID {
			writeMetric(points[n:i])
			n = i
			continue
		}
	}

	writeMetric(points[n:i])

	p.Stop()
}
