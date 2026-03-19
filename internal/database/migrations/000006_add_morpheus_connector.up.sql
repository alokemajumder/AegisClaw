-- Add Morpheus (NVIDIA GPU-accelerated security analytics) to the connector registry.
INSERT INTO connector_registry (id, connector_type, display_name, category, description, capabilities, config_schema, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    'morpheus',
    'NVIDIA Morpheus',
    'analytics',
    'GPU-accelerated security analytics via NVIDIA Morpheus (Triton + RAPIDS). Real-time log classification, anomaly detection, and sensitive information detection.',
    '["query_events"]',
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
    NOW(),
    NOW()
)
ON CONFLICT (connector_type) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    category = EXCLUDED.category,
    description = EXCLUDED.description,
    capabilities = EXCLUDED.capabilities,
    config_schema = EXCLUDED.config_schema,
    updated_at = NOW();
