-- Extend category CHECK constraint to include 'analytics'.
ALTER TABLE connector_registry DROP CONSTRAINT IF EXISTS connector_registry_category_check;
ALTER TABLE connector_registry ADD CONSTRAINT connector_registry_category_check
    CHECK (category IN ('siem','edr','itsm','identity','notification','cloud','analytics'));

-- Add Morpheus (NVIDIA GPU-accelerated security analytics) to the connector registry.
INSERT INTO connector_registry (connector_type, category, display_name, description, version, config_schema, capabilities, status)
VALUES (
    'morpheus',
    'analytics',
    'NVIDIA Morpheus',
    'GPU-accelerated security analytics via NVIDIA Morpheus (Triton + RAPIDS). Real-time log classification, anomaly detection, and sensitive information detection.',
    '1.0.0',
    '{
        "type": "object",
        "properties": {
            "triton_url":    {"type": "string", "title": "Triton URL",    "description": "Triton Inference Server endpoint (e.g. http://localhost:8000)"},
            "kafka_brokers": {"type": "string", "title": "Kafka Brokers", "description": "Comma-separated Kafka broker addresses for log ingestion"},
            "model_name":    {"type": "string", "title": "Model Name",    "description": "Morpheus pipeline model (sid-minibert, phishing-bert, anomaly-ae, etc.)", "default": "sid-minibert"},
            "api_key":       {"type": "string", "title": "API Key",       "description": "Optional API key for authenticated endpoints", "format": "password"}
        },
        "required": ["triton_url", "model_name"]
    }',
    '{"query_events"}',
    'available'
)
ON CONFLICT (connector_type) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    category = EXCLUDED.category,
    description = EXCLUDED.description,
    capabilities = EXCLUDED.capabilities,
    config_schema = EXCLUDED.config_schema;
