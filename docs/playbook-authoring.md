# Playbook Authoring Guide

## Overview

AegisClaw validation playbooks define the specific checks and emulations that agents execute. Playbooks are YAML files organized by tier and mapped to ATT&CK techniques.

## Playbook Structure

```yaml
# playbooks/tier0/siem-telemetry-health.yaml

id: t0-siem-telemetry-health
name: SIEM Telemetry Health Check
description: Verify that telemetry from endpoints is flowing to the SIEM within expected latency
version: 1
tier: 0
category: telemetry_health

# ATT&CK mapping (informational for Tier 0)
techniques: []

# Which asset types this playbook applies to
target_types:
  - server
  - endpoint

# Steps to execute
steps:
  - id: check_event_flow
    action: query_siem_events
    description: Query SIEM for recent events from target assets
    connector_category: siem
    parameters:
      time_range_minutes: 60
      expected_min_events: 1
    success_criteria:
      - "event_count >= expected_min_events"

  - id: check_event_latency
    action: measure_event_latency
    description: Measure the lag between event generation and SIEM indexing
    connector_category: siem
    parameters:
      max_acceptable_latency_seconds: 300
    success_criteria:
      - "avg_latency_seconds <= max_acceptable_latency_seconds"

# Expected outcome
expected_telemetry:
  - source: siem
    event_type: telemetry_health_check

# Cleanup (none needed for Tier 0 passive checks)
cleanup: null
```

## Tier 1 Playbook Example

```yaml
# playbooks/tier1/benign-file-creation.yaml

id: t1-benign-file-creation
name: Benign File Creation Detection
description: Create a safe marker file and verify it triggers expected EDR telemetry
version: 1
tier: 1
category: benign_emulation

# ATT&CK mapping
techniques:
  - T1059.001  # PowerShell
  - T1204.002  # User Execution: Malicious File

target_types:
  - endpoint
  - server

steps:
  - id: create_marker
    action: create_benign_marker
    description: Create a benign EICAR-like test file in a monitored directory
    execution_mode: sandbox
    parameters:
      filename: "aegisclaw-test-marker.txt"
      content: "X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"
      directory: "/tmp/aegisclaw-tests/"
    cleanup:
      action: delete_file
      parameters:
        path: "/tmp/aegisclaw-tests/aegisclaw-test-marker.txt"

  - id: wait_for_telemetry
    action: wait
    description: Wait for telemetry pipeline to process the event
    parameters:
      duration_seconds: 30

  - id: verify_edr_detection
    action: query_edr_events
    description: Check if the EDR detected the file creation
    connector_category: edr
    parameters:
      time_range_minutes: 5
      event_filter:
        type: file_creation
        filename: "aegisclaw-test-marker.txt"
    success_criteria:
      - "event_count >= 1"

  - id: verify_alert
    action: query_edr_alerts
    description: Check if an alert was generated
    connector_category: edr
    parameters:
      time_range_minutes: 10
      alert_filter:
        related_file: "aegisclaw-test-marker.txt"
    success_criteria:
      - "alert_count >= 1"

expected_telemetry:
  - source: edr
    event_type: file_creation
  - source: edr
    event_type: alert

cleanup:
  mandatory: true
  verify: true
  steps:
    - action: delete_file
      parameters:
        path: "/tmp/aegisclaw-tests/aegisclaw-test-marker.txt"
    - action: verify_file_deleted
      parameters:
        path: "/tmp/aegisclaw-tests/aegisclaw-test-marker.txt"
```

## Playbook Schema

Playbooks are validated against a JSON Schema. Key fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique identifier (prefix with tier: t0-, t1-, t2-) |
| `name` | string | Yes | Human-readable name |
| `description` | string | Yes | What this playbook validates |
| `version` | integer | Yes | Playbook version number |
| `tier` | integer | Yes | Governance tier (0, 1, or 2) |
| `category` | string | Yes | Playbook category |
| `techniques` | string[] | No | ATT&CK technique IDs |
| `target_types` | string[] | Yes | Asset types this applies to |
| `steps` | Step[] | Yes | Ordered list of execution steps |
| `expected_telemetry` | Telemetry[] | No | What telemetry should appear |
| `cleanup` | Cleanup | Yes for Tier 1+ | Cleanup instructions |

### Step Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Step identifier |
| `action` | string | Yes | Action to perform |
| `description` | string | Yes | What this step does |
| `connector_category` | string | No | Which connector category to use |
| `execution_mode` | string | No | `sandbox` for Runner execution |
| `parameters` | object | No | Step-specific parameters |
| `success_criteria` | string[] | No | Conditions for pass/fail |
| `cleanup` | object | No | Per-step cleanup |

## Categories

| Category | Description | Example Playbooks |
|----------|-------------|-------------------|
| `telemetry_health` | Verify telemetry pipeline health | SIEM event flow, EDR reporting |
| `config_posture` | Check security configurations | Policy compliance, rule deployment |
| `benign_emulation` | Safe test execution | File creation, process execution |
| `detection_validation` | Verify detection capability | Alert generation, response time |
| `identity_validation` | Identity/access checks | SSO config, MFA enforcement |

## Best Practices

1. **Start with Tier 0**: Always have passive health checks before emulations
2. **ATT&CK mapping**: Map every Tier 1+ playbook to ATT&CK techniques
3. **Cleanup is mandatory**: Every Tier 1+ playbook must define and verify cleanup
4. **Success criteria**: Be explicit about what constitutes pass/fail
5. **Expected telemetry**: Define what events should appear so the Validation Squad can verify
6. **Idempotency**: Playbooks should be safe to run multiple times
7. **Documentation**: Describe what the playbook does and why — this appears in reports
