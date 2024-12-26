# Log-exporter

- [Overall Design](#overall-design)
- [Documents](#documents)

## Overall Design

The log-exporter (or Logs-to-Metrics-exporter, LME) tool is designed to evaluate Prometheus metrics on the basis of the records retrieved from the Graylog server, New Relic, or Loki via API. The log-exporter can count logged events, evaluate average or sum values for numerical graylog fields or duration between logged events and provide results as prometheus metrics of different types (counter, gauge, histogram). The log-exporter can enrich the received data with use of regular expressions with templates and/or json-path. Any field can be used as the label for the metric. Log-exporter can push metrics to Victoria or Prometheus remote-write or metrics could be pulled from the `/metrics` endpoint by Prometheus, Victoria or any other consumer.

## Documents

 - [Installation notes](docs/installation-notes.md)
 - [User guide](docs/user-guide.md)