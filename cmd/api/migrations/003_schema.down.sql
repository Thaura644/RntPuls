DROP INDEX IF EXISTS idx_payment_confirmations_intent;
DROP INDEX IF EXISTS idx_payment_intents_lease;
DROP INDEX IF EXISTS idx_leases_tenant;
DROP INDEX IF EXISTS idx_units_property;

ALTER TABLE organizations DROP COLUMN IF EXISTS updated_at;
ALTER TABLE properties DROP COLUMN IF EXISTS updated_at;
ALTER TABLE units DROP COLUMN IF EXISTS updated_at;
ALTER TABLE tenants DROP COLUMN IF EXISTS updated_at;
ALTER TABLE leases DROP COLUMN IF EXISTS updated_at;
