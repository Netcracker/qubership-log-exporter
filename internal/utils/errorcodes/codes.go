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

package errorcodes

const (
    //Field name for error codes

    FIELD = "error_code"

    // LME inner business logic error codes LME-1000 - LME-1100

    //Prometheus client error codes LME-1000 - LME-1009

    LME_1000 = "LME-1000" // General Prometheus client error
    LME_1001 = "LME-1001" // Counter metric update error
    LME_1002 = "LME-1002" // Gauge metric update error
    LME_1003 = "LME-1003" // Histogram metric update error
    LME_1004 = "LME-1004" // Summary metric update error
    LME_1005 = "LME-1005" // Registry gather error

    //Enrich process error codes LME-1010 - LME-1019

    LME_1010 = "LME-1010" // Enriching error : Column insert error
    LME_1011 = "LME-1011" // Enriching error : Value evaluation error

    //Metric evaluation process error codes LME-1020 - LME-1039

    LME_1020 = "LME-1020" // General metric evaluation error

    //Metric format conversion error codes LME-1040 - LME-1049

    LME_1040 = "LME-1040" // General metric format conversion error
    LME_1041 = "LME-1041" // Metric Family to metric text format conversion error
    LME_1042 = "LME-1042" // Metric Family to prompb format conversion error

    // LME inner technical error codes LME-1600 - LME-1700

    LME_1600 = "LME-1600" // General LME technical error
    LME_1601 = "LME-1601" // Unexpected LME panic
    LME_1602 = "LME-1602" // Unexpected assertion error
    LME_1603 = "LME-1603" // Unexpected encryption error
    LME_1604 = "LME-1604" // Unexpected nil or empty object error
    LME_1605 = "LME-1605" // Graylog emulator technical error
    LME_1606 = "LME-1606" // Unexpected HTTP web-server error
    LME_1607 = "LME-1607" // Thread dump printing before exiting. Not an issue
    LME_1608 = "LME-1608" // Croniter registration error
    LME_1609 = "LME-1609" // To number conversion error

    // LME inner technical error codes for chan issues LME-1620 - LME-1630

    LME_1620 = "LME-1620" // General chan issues
    LME_1621 = "LME-1621" // Attempt to read from closed chan
    LME_1622 = "LME-1622" // Attempt to write to closed chan
    LME_1623 = "LME-1623" // Attempt to read from non-existent or nil chan
    LME_1624 = "LME-1624" // Attempt to write to non-existent or nil chan
    LME_1625 = "LME-1625" // Attempt to write to full chan

    // Graylog communication error codes LME-7100 - LME-7109

    LME_7100 = "LME-7100" // General Graylog communication error 
    LME_7101 = "LME-7101" // Graylog responded with error status code
    LME_7102 = "LME-7102" // Graylog responded with unexpected status code
    LME_7103 = "LME-7103" // Graylog response parsing error
    LME_7104 = "LME-7104" // Graylog response is not supported

    // Victoria communication error codes LME-7110 - LME-7119

    LME_7110 = "LME-7110" // General Victoria communication error
    LME_7111 = "LME-7111" // Victoria responded with error status code
    LME_7112 = "LME-7112" // Victoria responded with unexpected status code
    LME_7113 = "LME-7113" // Victoria response parsing error
    LME_7114 = "LME-7114" // Victoria response is not supported

    // Prometheus remote write communication error codes LME-7120 - LME-7129

    LME_7120 = "LME-7120" // General Prometheus remote write communication error
    LME_7121 = "LME-7121" // Prometheus remote write responded with error status code
    LME_7122 = "LME-7122" // Prometheus remote write responded with unexpected status code
    LME_7123 = "LME-7123" // Prometheus remote write response parsing error
    LME_7124 = "LME-7124" // Prometheus remote write response is not supported

    // Last timestamp service communication error codes LME-7130 - LME-7139

    LME_7130 = "LME-7130" // General Last timestamp service error
    LME_7131 = "LME-7131" // Last timestamp service responded with error status code
    LME_7132 = "LME-7132" // Last timestamp service responded with unexpected status code
    LME_7133 = "LME-7133" // Last timestamp service response parsing error
    LME_7134 = "LME-7134" // Last timestamp service response is not supported

    // New Relic communication error codes LME-7140 - LME-7149

    LME_7140 = "LME-7140" // General New Relic communication error
    LME_7141 = "LME-7141" // New Relic service responded with error status code
    LME_7142 = "LME-7142" // New Relic service responded with unexpected status code
    LME_7143 = "LME-7143" // New Relic service response parsing error
    LME_7144 = "LME-7144" // New Relic response is not supported

    // Consul communication error codes LME-7150 - LME-7159

    LME_7150 = "LME-7150" // General Consul communication error
    LME_7151 = "LME-7151" // Consul access error
    //LME_7152 = "LME-7152" // reserved
    //LME_7153 = "LME-7153" // reserved
    LME_7154 = "LME-7154" // Consul response is not supported

    // Invalid configuration error codes LME-8100 - LME-8200

    LME_8100 = "LME-8100" // General configuration error
    LME_8101 = "LME-8101" // Fatal error during configuration read, LME can not start up
    LME_8102 = "LME-8102" // Non-fatal inconsistency in yaml configuration. LME may work incorrectly
    LME_8103 = "LME-8103" // Datasource required credentials are missed
    LME_8104 = "LME-8104" // One of the yaml configuration parameters is set up incorrectly, the default value will be used for this.
    LME_8105 = "LME-8105" // Incorrect consul configuration
    LME_8106 = "LME-8106" // Incorrect graylog emulator configuration

)