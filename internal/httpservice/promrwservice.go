// Copyright 2024 Qubership
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httpservice

import (
	"bytes"
	"log_exporter/internal/config"
    ec "log_exporter/internal/utils/errorcodes"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
)

type PromRWService struct {
    endpoint string
    exportConfig *config.ExportConfig
    timeMillis int64
}

func NewPromWRService(exportConfig *config.ExportConfig) *PromRWService {
    endpoint := exportConfig.Host + exportConfig.Endpoint
    return &PromRWService{
        endpoint: endpoint,
        exportConfig: exportConfig,
    }
}

func (p *PromRWService) WriteMetrics(metricFamilies []*dto.MetricFamily, queryName string) (error, string) {
    protoBuffer, err := proto.Marshal(&prompb.WriteRequest{
        Timeseries: p.getProtoData(metricFamilies),
    })
    if err != nil {
        return fmt.Errorf("PromRWService : Marshaling error : %+v", err), ec.LME_1042
    }
    encodedProtoBuffer := snappy.Encode(nil, protoBuffer)
    encodedSize := len(encodedProtoBuffer)
    httpReq, err := http.NewRequest(http.MethodPost, p.endpoint, bytes.NewBuffer(encodedProtoBuffer))
    if err != nil {
        return err, ec.LME_7120
    }
    httpReq.Header.Add("X-Prometheus-Remote-Write-Version", "0.1.0")
    httpReq.Header.Add("Content-Encoding", "snappy")
    httpReq.Header.Set("Content-Type", "application/x-protobuf")
    httpReq.Header.Set("User-Agent", "log-exporter/1.0.0")
    if p.exportConfig.User != "" {
        httpReq.SetBasicAuth(p.exportConfig.User, p.exportConfig.Password)
    }

    var transport http.RoundTripper = &http.Transport{
        DialContext: (&net.Dialer{
            Timeout: p.exportConfig.ConnectionTimeout,
        }).DialContext,
        TLSClientConfig: p.exportConfig.TlsConfig,
    }
    client := http.Client{
        Transport: transport,
        Timeout:   p.exportConfig.ConnectionTimeout,
    }
    log.Infof("PromRWService : For query %v sending request, size of encoded message is %v bytes", queryName, encodedSize)
    resp, err := client.Do(httpReq)
    if err != nil {
        return fmt.Errorf("PromRWService : Error sending remote-write request: %w", err), ec.LME_7120
    }    
    defer resp.Body.Close()

    status := resp.StatusCode
    if status / 100 != 2 {
        msg, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("PromRWService : Got http status %v from remote-write, response body is %v", status, string(msg)), ec.LME_7122
    }

    log.Infof("PromRWService : For query %v metrics were pushed successfully, got status %v", queryName, resp.Status)
    return nil, ""
}

func (p *PromRWService) getProtoData(metricFamilies []*dto.MetricFamily) []prompb.TimeSeries {
    p.timeMillis = time.Now().UnixMilli()
    res := make([]prompb.TimeSeries, 0)
    for _, mf := range metricFamilies {
        timeSeries, err := p.metricFamilyToPrompb(mf)
        if err != nil {
            log.WithField(ec.FIELD, ec.LME_1042).Errorf("Error converting metricFamily to prompb timeSeries : %+v", err)
        }
        res = append(res, timeSeries...)
    }
    return res
}

func (p *PromRWService) metricFamilyToPrompb(in *dto.MetricFamily) ([]prompb.TimeSeries, error) {
    res := make([]prompb.TimeSeries, 0, 100)
    if len(in.Metric) == 0 {
        return res, fmt.Errorf("MetricFamily has no metrics: %+v", in)
    }
    name := in.GetName()
    if name == "" {
        return res, fmt.Errorf("MetricFamily has no name: %+v", in)
    }

    metricType := in.GetType()
    for _, metric := range in.Metric {
        switch metricType {
        case dto.MetricType_COUNTER:
            if metric.Counter == nil {
                return res, fmt.Errorf("Expected counter in metric %v : %+v", name, metric)
            }
            res = append(res, p.getTimeSeries(name, "", metric, "", 0, metric.Counter.GetValue()))
        case dto.MetricType_GAUGE:
            if metric.Gauge == nil {
                return res, fmt.Errorf("Expected gauge in metric %v : %+v", name, metric)
            }
            res = append(res, p.getTimeSeries(name, "", metric, "", 0, metric.Gauge.GetValue()))
        case dto.MetricType_UNTYPED:
            if metric.Untyped == nil {
                return res, fmt.Errorf("Expected untyped in metric %v : %+v", name, metric)
            }
            res = append(res, p.getTimeSeries(name, "", metric, "", 0, metric.Untyped.GetValue()))
        case dto.MetricType_SUMMARY:
            if metric.Summary == nil {
                return res, fmt.Errorf("Expected summary in metric %v : %+v", name, metric)
            }
            for _, q := range metric.Summary.Quantile {
                res = append(res, p.getTimeSeries(name, "", metric, model.QuantileLabel, q.GetQuantile(), q.GetValue()))
            }
            res = append(res, p.getTimeSeries(name, "_sum", metric, "", 0, metric.Summary.GetSampleSum()))
            res = append(res, p.getTimeSeries(name, "_count", metric, "", 0, float64(metric.Summary.GetSampleCount())))
        case dto.MetricType_HISTOGRAM:
            if metric.Histogram == nil {
                return res, fmt.Errorf("Expected histogram in metric %v : %+v", name, metric)
            }
            infSeen := false
            for _, b := range metric.Histogram.Bucket {
                res = append(res, p.getTimeSeries(name, "_bucket", metric, model.BucketLabel, b.GetUpperBound(), float64(b.GetCumulativeCount())))
                if math.IsInf(b.GetUpperBound(), +1) {
                    infSeen = true
                }
            }
            if !infSeen {
                res = append(res, p.getTimeSeries(name, "_bucket", metric, model.BucketLabel, math.Inf(+1), float64(metric.Histogram.GetSampleCount())))
            }
            res = append(res, p.getTimeSeries(name, "_sum", metric, "", 0, metric.Histogram.GetSampleSum()))
            res = append(res, p.getTimeSeries(name, "_count", metric, "", 0, float64(metric.Histogram.GetSampleCount())))
        default:
            return res, fmt.Errorf("Unexpected type in metric %v : %+v", name, metric)
        }
    }
    return res, nil
}

func (p *PromRWService) getTimeSeries(name, suffix string, metric *dto.Metric, additionalLabelName string, additionalLabelValue float64, value float64) (prompb.TimeSeries) {
    res := prompb.TimeSeries{}
    res.Labels = make([]prompb.Label, 0)
    res.Labels = append(res.Labels, prompb.Label{Name: "__name__", Value: name + suffix})

    for _, labelPair := range metric.Label {
        res.Labels = append(res.Labels, prompb.Label{Name: *labelPair.Name, Value: *labelPair.Value})
    }
    if additionalLabelName != "" {
        res.Labels = append(res.Labels, prompb.Label{Name: additionalLabelName, Value: fmt.Sprintf("%v", additionalLabelValue)})
    }
    if metric.TimestampMs != nil {
        res.Samples = []prompb.Sample{{
            Timestamp: *metric.TimestampMs,
            Value:     value,
        }}
    } else {
        res.Samples = []prompb.Sample{{
            Timestamp: p.timeMillis,
            Value:     value,
        }}
    }
    return res
}