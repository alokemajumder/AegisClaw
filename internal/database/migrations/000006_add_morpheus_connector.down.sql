DELETE FROM connector_registry WHERE connector_type = 'morpheus';

-- Restore original category CHECK constraint without 'analytics'.
ALTER TABLE connector_registry DROP CONSTRAINT IF EXISTS connector_registry_category_check;
ALTER TABLE connector_registry ADD CONSTRAINT connector_registry_category_check
    CHECK (category IN ('siem','edr','itsm','identity','notification','cloud'));
