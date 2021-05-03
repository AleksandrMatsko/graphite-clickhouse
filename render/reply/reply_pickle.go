package reply

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/lomik/graphite-clickhouse/helper/point"
	"github.com/lomik/graphite-clickhouse/pkg/scope"
	"github.com/lomik/graphite-clickhouse/render/data"
	graphitePickle "github.com/lomik/graphite-pickle"
	"go.uber.org/zap"
)

type Pickle struct{}

func (*Pickle) ParseRequest(r *http.Request) (fetchRequests data.MultiFetchRequest, err error) {
	return parseRequestForms(r)
}

func (*Pickle) Reply(w http.ResponseWriter, r *http.Request, multiData data.CHResponses) {
	var pickleTime time.Duration
	// Pickle response always contain single request/response
	data := multiData[0].Data
	from := uint32(multiData[0].From)
	until := uint32(multiData[0].Until)

	logger := scope.Logger(r.Context())

	defer func() {
		logger.Debug("pickle",
			zap.String("runtime", pickleTime.String()),
			zap.Duration("runtime_ns", pickleTime),
		)
	}()

	if data.Len() == 0 {
		w.Write(graphitePickle.EmptyList)
		return
	}

	writer := bufio.NewWriterSize(w, 1024*1024)
	p := graphitePickle.NewWriter(writer)
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

		start, end, _, getValue := point.FillNulls(points, from, until, step)

		p.String("values")
		p.List()
		for {
			value, err := getValue()
			if err != nil {
				if errors.Is(err, point.ErrTimeGreaterStop) {
					break
				}
				// if err is not point.ErrTimeGreaterStop, the points are corrupted
				return
			}
			if !math.IsNaN(value) {
				p.AppendFloat64(value)
				continue
			}
			p.AppendNulls(1)
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

	writeMetric := func(points []point.Point) error {
		metricName := data.MetricName(points[0].MetricID)
		step, err := data.GetStep(points[0].MetricID)
		if err != nil {
			logger.Error("fail to get step", zap.Error(err))
			http.Error(w, fmt.Sprintf("failed to get step for metric: %v", data.MetricName(points[0].MetricID)), http.StatusInternalServerError)
			return err
		}
		for _, a := range data.AM.Get(metricName) {
			writeAlias(a.DisplayName, a.Target, points, step)
		}
		return nil
	}

	nextMetric := data.GroupByMetric()
	for {
		points := nextMetric()
		if len(points) == 0 {
			break
		}
		if err := writeMetric(points); err != nil {
			return
		}
	}

	p.Stop()
}
