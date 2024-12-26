- [Logs2Metrics Exporter (LME) Installation Notes](#logs2metrics-exporter-installation-notes)
    - [Prerequisites](#prerequisites)
        - [Graylog Instance (optional)](#graylog-instance-optional)
        - [Access to the New Relic (optional)](#access-to-the-new-relic-optional)
        - [Access to the Loki (optional)](#access-to-the-loki-optional)
        - [Victoria Instance (optional)](#victoria-instance-optional)
        - [Prometheus Instance (optional)](#prometheus-instance-optional)
        - [Consul KV-storage (optional)](#consul-kv-storage-optional)
        - [Third-Party Software](#third-party-software)
    - [Deployment Configuration](#deployment-configuration)
        - [Configuring the Graylog Host](#configuring-the-graylog-host)
        - [Configuring the New Relic Service](#configuring-the-new-relic-service)
        - [Configuring the Loki Service](#configuring-the-loki-service)
        - [Configuring the Metrics Pushing](#configuring-the-metrics-pushing)
        - [Configuring HA for Push Metrics](#configuring-ha-for-push-metrics)
        - [Configuring Metrics Pulling in Cloud Environments](#configuring-metrics-pulling-in-cloud-environments)
    - [Full list of Parameters](#full-list-of-parameters)
        - [General Helm Parameters](#general-helm-parameters)
        - [Technical Helm Parameters](#technical-helm-parameters)

# Logs2Metrics Exporter Installation Notes

This document describes the installation process for the log-exporter service.

The log-exporter deployment artifact contains the following list of services.

| Service Name      | Description                          |
|-------------------|--------------------------------------|
| log-exporter      | Service for log-exporter application |

## Prerequisites

### Graylog Instance (optional)

Log-exporter can connect to the Graylog Server via REST API with Basic auth. LME usually uses the Graylog as a source of data.

### Access to the New Relic (optional)

Log-exporter can connect to the New Relic via New Relic Insights query API. LME can use the New Relic as a source of data.

### Access to the Loki (optional)

Log-exporter can connect to the Loki via Loki API. LME can use the Loki as a source of data.

### Victoria Instance (optional)

In the push mode, log-exporter can push metrics to the Victoria Metrics Server via API with Basic auth. Log-exporter needs the vmagent service available in the push mode and also the vmauth service available in the push mode with high availability support.
In the pull mode, Victoria might pull the data from the log-exporter metrics endpoint via the system monitor. High availability is not supported for pull mode.

### Prometheus Instance (optional)

In the push mode, log-exporter can push metrics to the Prometheus Server via the remote-write API. Log-exporter needs the carbon-clickhouse service available in the push mode and also the graphite-clickhouse service available in the push mode with high availability support.
In the pull mode, Prometheus might pull the data from the log-exporter metrics endpoint via the system monitor. High availability is not supported for pull mode.

### Consul KV-storage (optional)

LME can use Consul KV-storage for LME log-level runtime modification.

### Third-Party Software

| Name                   | Version                                                  |
|------------------------|----------------------------------------------------------|
| OpenShift/Kubernetes   | 3.11 (Enterprise or Origin edition)/1.20+                |
| Graylog                | v3 or higher                                             |
| Victoria Metrics       | >=v1.68.0 (probably also works fine on earlier versions) |

## Deployment Configuration

Log-exporter can be configured to communicate with the following services:  
- Graylog service via Graylog REST API.
- New Relic service via the New Relic Insights query API.
- Loki service via the Loki API.
- Victoria Metrics via REST API for vmagent and/or vmauth.
- Prometheus remote-write via binary interfaces for carbon-clickhouse and/or graphite-clickhouse.
- Any service (usually it is Prometheus) can pull the metrics from :8081/metrics endpoint of the log-exporter.
- Consul KV-storage via Consul API (can be used for LME log-level runtime management)  
Note that at least one Graylog or New Relic or Loki service is required for the log-exporter startup. Other services are optional.  
### Configuring the Graylog Service
The Graylog URL is usually set up by the helm parameter GRAYLOG_UI_URL in the format `<protocol>://<ip_or_dns>:<port>`. The port may be omitted. If the URL from the helm parameter GRAYLOG_UI_URL cannot be used, it can be overwritten by the helm parameter GRAYLOG_HOST. If both helm parameters GRAYLOG_UI_URL and GRAYLOG_HOST are empty, the default Graylog URL https://<area>graylog-logging.${CLOUD_PUBLIC_HOST} is used, where ${CLOUD_PUBLIC_HOST} is the value of the helm parameter CLOUD_PUBLIC_HOST. The helm parameters CSE_GRAYLOG_USER and CSE_GRAYLOG_PASSWORD set up the credentials for Graylog REST API Basic auth. If the credentials from the helm parameters cannot be used, the credentials can be overwritten by the helm parameters GRAYLOG_USER and GRAYLOG_PASSWORD. The Graylog credentials helm parameters may be omitted if the credentials are already specified in the Kubernetes secret. The Kubernetes secret is updated during the deployment if the Graylog credentials are specified in helm. As a result, the Graylog credentials configuration can usually be skipped in helm if the credentials have not been changed since the latest installation. However, if the Graylog credentials have been changed since the latest installation or it is the first installation in the cloud, the Graylog credentials must be specified. If the Graylog configuration is specified correctly, the log-exporter exposes the Prometheus metrics on the endpoint :8081/metrics for pulling.

### Configuring the New Relic Service
To configure New Relic as the datasource, set up helm parameters LME_DATASOURCE_TYPE to "newrelic" and LME_NEWRELIC_URL as the URL of the New Relic Insights query API in the format: `<protocol>://<ip_or_dns>:<port>`. Also it is usually required to specify New Relic credentials in helm parameters: in LME_NEWRELIC_ACCOUNT_ID, specify  the New Relic Account Id and in LME_NEWRELIC_X_QUERY_KEY, specify the New Relic X-Query-Key. The New Relic credentials may be omitted only if the credentials are already specified in the Kubernetes secret.  

### Configuring the Loki Service
To configure Loki as the datasource, set up helm parameters LME_DATASOURCE_TYPE to "loki" and LME_LOKI_URL as the URL of the Loki API, usually in the following format: `<protocol>://<ip_or_dns>`. Also it may be required to specify Loki credentials in helm parameters: in LME_LOKI_USER, specify the Loki API user and in LME_LOKI_PASSWORD, specify the Loki API password. The Loki credentials may be omitted if the credentials are already specified in the Kubernetes secret.  

### Configuring the Metrics Pushing
Log-exporter can be configured to push metrics to one of the following services: Victoria Metrics or Prometheus remote-write binary interface. Pushing is enabled by default. To disable pushing, helm parameter LME_MODE must be set to "pull".

#### Victoria Metrics
For pushing to the Victoria Metrics, the MONITORING_TYPE parameter must be specified in helm and set to "VictoriaDB". By default, metrics are pushed to the URL http://<area>vmagent-k8s.${MONITORING_NAMESPACE}:8429. The default value of the helm parameter MONITORING_NAMESPACE is "platform-monitoring". As a result, by default metrics are pushed to http://<area>vmagent-k8s.platform-monitoring:8429. For pushing to an arbitrary Victoria Metrics instance, the VICTORIA_HOST parameter must be specified in helm, which is the vmagent service URL in the format: `<protocol>://<ip_or_dns>:<port>`. The port may be omitted in this case. helm parameters VICTORIA_USER and VICTORIA_PASSWORD are used to set up basic auth credentials for vmagent. The Kubernetes secret is updated during the deployment if VICTORIA_USER and VICTORIA_PASSWORD are specified in helm. As a result, VICTORIA_USER and VICTORIA_PASSWORD can usually be skipped in helm if the credentials have not been changed since the latest installation. However, if the Graylog credentials have been changed since the latest installation or it is the first installation on the cloud, VICTORIA_USER and VICTORIA_PASSWORD must be specified.  

#### Prometheus Remote-Write Binary Interface
For pushing to the *Prometheus remote-write binary interface*, parameter MONITORING_TYPE must be specified in helm and set to "Prometheus". By default, metrics are pushed to the URL http://<area>vmagent-k8s.${MONITORING_NAMESPACE}:8429 . Helm parameter MONITORING_NAMESPACE default value is "platform-monitoring", as a result, by default metrics are pushed to the URL http://<area>vmagent-k8s.platform-monitoring:8429 . For pushing to an arbitrary *Prometheus remote-write binary interface* instance, parameter PROMRW_HOST must be specified in helm, which is the carbon-clickhouse service URL in the format: `<protocol>://<ip_or_dns>:<port>`. The port may be omitted in this case. Carbon-clickhouse usually does not use authentication; however, the PROMRW_USER and PROMRW_PASSWORD parameters are available in helm. Usually, they are empty in this case.  
 
### Configuring HA for Push Metrics
By default, HA is enabled in the push mode and log-exporter retrieves last-timestamp information from the host http://<area>vmsingle-k8s.${Values.MONITORING_NAMESPACE}:8429. The default value of the helm parameter MONITORING_NAMESPACE is "platform-monitoring". As a result, by default last-timestamp information is retrieved from http://<area>vmsingle-k8s.platform-monitoring:8429. For retrieving the last-timestamp information from an arbitrary host, helm parameter MONITORING_EXT_MONITORING_QUERY_URL must also be specified in the format: `<protocol>://<ip_or_dns>:<port>`. The port may be omitted in this case. This host is used for retrieving the last timestamp from the metric storage (Victoria vmauth or graphite-clickhouse). helm parameters CSE_EXTERNAL_MONITORING_USER and CSE_EXTERNAL_MONITORING_PASSWORD can be set for updating the credentials in Kubernetes secret. For graphite-clickhouse, credentials are usually not needed. If there is a need to disable HA in the push mode, MONITORING_EXT_MONITORING_QUERY_URL must be set to "None".

### Configuring Metrics Pulling in Cloud Environments
In cloud environments, the Service Monitor must be activated for metrics pulling. To activate the Service Monitor, set the helm parameter LME_MODE to "pull" (by default LME_MODE is set to "push" and Service Monitor is disabled). If LME_MODE is set to "pull", out-of-box pushing is disabled.

## Full List of Parameters
The following is the full list of the helm parameters that defines the log-exporter deployment mode and is used during the installation. Currently there are no mandatory helm parameters.  

### General helm Parameters

| Parameter                                         | Mand<br>atory | Default                                               | Value Example                             |Description                                                                                                                                                                                         |
|---------------------------------------------------|:-------------:|-------------------------------------------------------|-------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| GRAYLOG_UI_URL                                    |     N         | https://<area>graylog.logging.<br>${CLOUD_PUBLIC_HOST}/     | https://<area><GRAYLOG_<br>SERVER_URL> |The Graylog service URL to which the log-exporter is connected.                                                                                             |
| CSE_GRAYLOG_USER                                  |     N         | -                                                     | user                                      |The Graylog user. May be omitted if it is already set in the secret.                                                                                           |
| CSE_GRAYLOG_PASSWORD                              |     N         | -                                                     | password                                  |The Graylog password. May be omitted if it is already set in the secret.                                                                                       |
| GRAYLOG_HOST                                      |     N         | -                                                     | https://<area><GRAYLOG_<br>SERVER_URL>    |The parameter allows to overwrite  GRAYLOG_UI_URL.                                                                                                                                                  |
| GRAYLOG_USER                                      |     N         | -                                                     | user                                      |The parameter allows to overwrite  CSE_GRAYLOG_USER.                                                                                                                                                |
| GRAYLOG_PASSWORD                                  |     N         | -                                                     | password                                  |The parameter allows to overwrite  CSE_GRAYLOG_PASSWORD.                                                                                                                                            |
| LME_DATASOURCE_TYPE                               |     N         | graylog                                               | graylog                                   |LME datasource type. Possible values are "graylog", "loki" and "newrelic". Configure this only if you are fully aware of Loki and New Relic integration functionality.                              |
| LME_NEWRELIC_URL                                  |     N         | -                                                     | https://<area><NEWRELIC_<br>SERVER_URL>   |URL of New Relic Insights query API.                                                                                                                                                                |
| LME_NEWRELIC_ACCOUNT_ID                           |     N         | -                                                     |                                           |New Relic Account ID.                                                                                                                                                                               |
| LME_NEWRELIC_X_QUERY_KEY                          |     N         | -                                                     |                                           |New Relic X-Query-Key.                                                                                                                                                                              |
| LME_LOKI_URL                                      |     N         | -                                                     | https://<area><LOKI_<br>SERVER_URL>       |The Loki service URL to which the log-exporter is connected.                                                                                                                                        |
| LME_LOKI_USER                                     |     N         | -                                                     | user                                      |The Loki user. May be omitted if it is already set in the secret.                                                                                                                                   |
| LME_LOKI_PASSWORD                                 |     N         | -                                                     | password                                  |The Loki password. May be omitted if it is already set in the secret.                                                                                                                               |
| VICTORIA_HOST                                     |     N         | http://<area>vmagent-k8s.<br>platform-monitoring:8429 | https://<area><VICTORIA_<br>VMAGENT_URL>:<<area>PORT> |The Victoria vmagent service URL (port is usually 8429).                                                                                                                                |
| VICTORIA_USER                                     |     N         | -                                                     | user                                      |The Victoria vmagent user. May be omitted if it is already set in the secret.                                                                                                                       |
| VICTORIA_PASSWORD                                 |     N         | -                                                     | password                                  |The Victoria vmagent password. May be omitted if it is already set in the secret.                                                                                                                   |
| PROMRW_HOST                                       |     N         | http://<area>vmagent-k8s.<br>platform-monitoring:8429 | https://<area><PROMRW_<br>URL>:<<area>PORT>           |The Carbon-clickhouse service URL.                                                                                                                                                      |
| PROMRW_USER                                       |     N         | -                                                     | user                                      |The Carbon-clickhouse user. May be omitted if it is already set in the secret. Usually empty.                                                                                                       |
| PROMRW_PASSWORD                                   |     N         | -                                                     | password                                  |The Carbon-clickhouse password. May be omitted if it is already set in the secret. Usually empty.                                                                                                   |
| MONITORING_NAMESPACE                              |     N         | platform-monitoring                                   | platform-monitoring                       |The name of the monitoring namespace. The parameter is used for evaluating the default values of VICTORIA_HOST, PROMRW_HOST, and MONITORING_EXT_MONITORING_QUERY_URL.  |
| MONITORING_TYPE                                   |     N         | VictoriaDB                                            | VictoriaDB                                |The monitoring type on the cloud. Possible values are "VictoriaDB" (data is pushed plain-text to VMAgent) and "Prometheus" (data is pushed in promrw binary format).  |
| MONITORING_EXT_<br>MONITORING_QUERY_URL           |     N         | http://<area>vmsingle-k8s.<br>platform-monitoring:8429 | https://<area><VICTORIA_<br>VMAUTH_URL>:<<area>PORT>  |The Victoria vmauth URL (port is usually 8427 or 8429) OR graphite-clickhouse service URL for PROMRW case.                                        |
| CSE_EXTERNAL_<br>MONITORING_USER                  |     N         | -                                                     | user                                      |The MONITORING_EXT_MONITORING_QUERY_URL user. May be omitted if it is already set in the secret.                                                               |
| CSE_EXTERNAL_<br>MONITORING_PASSWORD              |     N         | -                                                     | password                                  |The MONITORING_EXT_MONITORING_QUERY_URL password. May be omitted if it is already set in the secret.                                                           |
| LAST_TIMESTAMP_HOST                               |     N         | -                                                     | https://<area><VICTORIA_<br>VMAUTH_URL>:<<area>PORT>  |The parameter allows to overwrite MONITORING_EXT_MONITORING_QUERY_URL.                                                                                                                  |
| LAST_TIMESTAMP_USER                               |     N         | -                                                     | user                                      |The parameter allows to overwrite CSE_EXTERNAL_MONITORING_USER.                                                                                                                                     |
| LAST_TIMESTAMP_<br>PASSWORD                       |     N         | -                                                     | password                                  |The parameter allows to overwrite CSE_EXTERNAL_MONITORING_PASSWORD.                                                                                                                                 |
| LME_MODE                                          |     N         | push                                                  | pull                                      |The parameter specifies the LME working mode. Set to "pull" if LME is not expected to push metrics to Prometheus, Victoria, or any other system and the LME is expected to work in the pull mode only. |
| CONFIG_MAP                                        |     N         | -                                                     |     |The config map content. If this variable is set, the out-of-box configuration is ignored.                                                                                                           |
| CONFIG_QUERIES                                    |     N         | -                                                     |     |Additional content of the config map queries section. The out-of-box configuration is not ignored.                                                                                                  |
| CONFIG_METRICS                                    |     N         | -                                                     |     |Additional content of the config map metrics section. The out-of-box configuration is not ignored.                                                                                                  |
| PUSH_CLOUD_LABELS                                 |     N         | -                                                     |                                           |A map of additional labels to be added to all pushed metrics.                                                                                                                                       |
| LOG_LEVEL                                         |     N         | -                                                     | debug                                     |The LME log-level. The possible values are "trace", "debug", "info", "warn" (or "warning"), "error", "fatal", and "panic".                                                                          |
| CONSUL_URL                                        |     N         | http://<area>consul-server.<br>consul:8500            |                                           |Consul URL. LME uses Consul to extract log.level property value for log-level runtime modification.                                                            |
| CONSUL_ADMIN_TOKEN                                |     N         | -                                                     | 01234567-0123-4123-4123-0123456789ab      |Consul token for the Consul CONSUL_URL. The token must have enough rights to read log.level property value.                                                    |
| LME_CONSUL_ENABLED                                |     N         | false                                                 | true                                      |Set to "true" if there is a need to modify LME log-level in runtime via Consul.                                                                                                                     |
| LME_CONSUL_CHECK_PERIOD                           |     N         | 1m                                                    | 1m30s                                     |The parameter specifies the period between calls to Consul in golang time.ParseDuration format. Less period means more frequent calls and faster reaction to log.level modification, but higher load on Consul. |
| LME_CONSUL_LOG_LEVEL_PATH                         |     N         | config/<namespace><br>/lme/log.level                  | debug                                     |The parameter specifies the path to the log.level property in the Consul key-value storage if the default path is not suitable.                                                                     |
| LME_LOG_FORMAT                                    |     N         | cloud                                                 | json                                      |The LME log format. The possible values are "cloud", "json", "text". "cloud" is compatible with logging guide, "text" is the standard logrus library format, and "json" has the same content as "text", but in json. |
| DEPLOYMENT_<br>STRATEGY_TYPE                      |     N         |                                                       |                                           | Kubernetes rolling update deployment strategy. Possible values are "recreate", "best_effort_controlled_rollout", "ramped_slow_rollout", and "custom_rollout".                                      |
| DEPLOYMENT_<br>STRATEGY_MAXSURGE                  |     N         | 25%                                                   | 25%                                       | The parameter sets maxSurge if DEPLOYMENT_STRATEGY_TYPE is "custom_rollout".                                                                                                                       |
| DEPLOYMENT_<br>STRATEGY_<br>MAXUNAVAILABLE        |     N         | 25%                                                   | 25%                                       | The parameter sets maxUnavailable if DEPLOYMENT_STRATEGY_TYPE is "custom_rollout".                                                                                                                 |

### Technical helm Parameters

All of the following parameters are optional.  

| Parameter                    | Default | Value Example |Description                                                                                                        |
|------------------------------|---------|---------------|-------------------------------------------------------------------------------------------------------------------|
| LME_APPLICATION_NAME         | log-exporter | log-exporter |The LME application name. The parameter is required only if several log-exporter instances need to be installed in the same namespace (for example, one instance is communicating with Graylog, and the other is communicating with New Relic).|
| LME_LTS_RETRY_COUNT          | 5       | 5             |Number of attempts to extract the last timestamp from Prometheus. If the value is "0", the last timestamp is not extracted; if the value is "1", retry is disabled. |
| LME_LTS_RETRY_PERIOD         | 10s     | 10s           |The time period between attempts to extract the last timestamp from Prometheus or Victoria.                        |
| LME_DATASOURCE_RETRY         | true    | true          |The parameter allows to enable or disable the retry mechanism for the datasource (Graylog or New Relic).           |
| LME_DATASOURCE_RETRY_PERIOD  | 5s      | 5s            |The time period between attempts to extract the log records from the datasource (Graylog or New Relic).            |
| LME_PUSH_RETRY               | true    | true          |The parameter allows to enable or disable the retry mechanism for Prometheus or Victoria in the push mode.         |
| LME_PUSH_RETRY_PERIOD        | 5s      | 5s            |The time period between attempts to push the metrics to Prometheus or Victoria.                                    |
| GRAYLOG_EMULATOR_<br>ENABLED | false   | true          |To enable Graylog emulator, set this parameter to "true". Must be used only on the dev and test environments.      |
| GRAYLOG_EMULATOR_<br>DATA    | -       | -             |The list of Graylog emulator responses. If the parameter is not set, the default data set is used. The parameter is used only if GRAYLOG_EMULATOR_ENABLED is set to "true". |

**NOTE:** 
- Graylog user, which is set in the helm parameter CSE_GRAYLOG_USER, must have the following permissions:  
clusterconfigentry:read, indexercluster:read, messagecount:read, journal:read, messages:analyze, inputs:read, metrics:read, fieldnames:read, buffers:read, system:read, jvmstats:read, decorators:read, throughput:read, messages:read, searches:relative, searches:absolute, searches:keyword.
- If the helm parameter MONITORING_TYPE is set to "VictoriaDB" and basic auth for the vmagent service is needed, the Victoria user, which is set in the helm parameter VICTORIA_USER, must have the right to import metrics to Victoria via API /api/v1/import/prometheus.
- If the helm parameter MONITORING_TYPE is set to "Prometheus" and basic auth for the carbon-clickhouse service is needed, the Prometheus remote-write user, which is set in the helm parameter PROMRW_HOST, must have the right to write metrics via API /api/v1/write.