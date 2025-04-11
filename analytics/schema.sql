CREATE TYPE "alert_states" AS ENUM (
  'unprocessed',
  'active',
  'firing',
  'suppressed',
  'deleted'
);

CREATE TYPE "suppressed_reason" AS ENUM ('silenced', 'inhibited');

CREATE TABLE "alerts" (
  "id" UUID PRIMARY KEY,
  "created_at" timestamp DEFAULT (now ()) NOT NUll,
  "fingerprint" string UNIQUE NOT NUll,
  "alertname" string NOT NUll
);

CREATE TABLE "alert_states" (
  "id" UUID PRIMARY KEY,
  "created_at" timestamp DEFAULT (now ()) NOT NUll,
  "alert" STRING REFERENCES alerts (fingerprint) NOT NUll,
  "state" alert_states NOT NUll,
  "suppressed_by" string REFERENCES alerts (fingerprint),
  "suppressed_reason" suppressed_reason
);

CREATE TABLE "labels" (
  "id" UUID PRIMARY KEY,
  "key" string NOT NUll,
  "value" string NOT NUll
);

CREATE TABLE "annotations" (
  "id" UUID PRIMARY KEY,
  "key" string NOT NUll,
  "value" string NOT NUll
);

CREATE TABLE "alert_label_set" (
  "id" UUID PRIMARY KEY,
  "alert" string REFERENCES alerts (fingerprint) NOT NUll,
  "label" UUID REFERENCES labels (id) NOT NUll
);

CREATE TABLE "alert_annotation_set" (
  "id" UUID PRIMARY KEY,
  "alert" string REFERENCES alerts (fingerprint) NOT NULL,
  "annotation" UUID REFERENCES annotations (id) NOT NULL
);

CREATE UNIQUE INDEX labels_key_value_unique ON "labels" ("key", "value");

CREATE UNIQUE INDEX anno_key_value_unique ON "annotations" ("key", "value");

CREATE UNIQUE INDEX alert_lbl_set_alert_lbl_unique ON "alert_label_set" ("alert", "label");

CREATE UNIQUE INDEX alert_anno_set_alert_lbl_unique ON "alert_annotation_set" ("alert", "annotation");
