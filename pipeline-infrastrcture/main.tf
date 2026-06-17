terraform {
  required_version = ">= 1.3.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0.0, < 6.0.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}


# ------------------------------------------------------------------------------
# Data Sources
# ------------------------------------------------------------------------------
data "google_project" "project" {
  project_id = var.project_id
}

locals {
  name_prefix            = "${var.environment}-platform"
  pubsub_service_account = "service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"

  # ----------------------------------------------------------------------------
  # Load raw definitions directly from the schema-managment workspace.
  # This completely avoids duplicating proto files in the pipeline infrastructure.
  # ----------------------------------------------------------------------------
  raw_login_proto       = file("${path.module}/../schema-managment/proto/platform/events/v1/login.proto")
  raw_level_proto       = file("${path.module}/../schema-managment/proto/platform/events/v1/level.proto")
  raw_transaction_proto = file("${path.module}/../schema-managment/proto/platform/events/v1/transaction.proto")

  # Map each schema key to its dynamically resolved schema definition.
  # Standalone schemas (login, level, transaction) are read directly from the schema-management workspace.
  schema_definitions = {
    "login"       = local.raw_login_proto
    "level"       = local.raw_level_proto
    "transaction" = local.raw_transaction_proto
  }
}

# ------------------------------------------------------------------------------
# Google Cloud APIs Activation
# ------------------------------------------------------------------------------
resource "google_project_service" "apis" {
  for_each = toset([
    "pubsub.googleapis.com",
    "bigquery.googleapis.com",
    "run.googleapis.com",
    "iam.googleapis.com"
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

# ------------------------------------------------------------------------------
# Pub/Sub Schemas
# ------------------------------------------------------------------------------
resource "google_pubsub_schema" "schemas" {
  for_each = var.event_topics

  name = "${local.name_prefix}-${each.key}-schema"
  type = "PROTOCOL_BUFFER"

  # Reads the dynamically loaded schema definitions without file duplication!
  definition = local.schema_definitions[each.key]

  # Ensure APIs are enabled before creating the schemas
  depends_on = [google_project_service.apis]
}

# ------------------------------------------------------------------------------
# Pub/Sub Topics
# ------------------------------------------------------------------------------
resource "google_pubsub_topic" "topics" {
  for_each = var.event_topics

  name = "${local.name_prefix}-${each.key}"

  # Ensure the schema exists before attaching it to the topic
  depends_on = [google_pubsub_schema.schemas]

  schema_settings {
    schema   = google_pubsub_schema.schemas[each.key].id
    encoding = var.schema_encoding
  }

  labels = {
    environment  = var.environment
    managed_by   = "terraform"
    message_type = replace(lower(each.value.message_type), ".", "-")
  }
}

# ------------------------------------------------------------------------------
# BigQuery Schema Definitions (Generated dynamically by Buf, with Event fallback)
# ------------------------------------------------------------------------------
locals {
  bigquery_schemas = {
    "login"       = file("${path.module}/../schema-managment/gen/bq/platform.events.v1.LoginEvent.TableSchema.json")
    "level"       = file("${path.module}/../schema-managment/gen/bq/platform.events.v1.LevelEvent.TableSchema.json")
    "transaction" = file("${path.module}/../schema-managment/gen/bq/platform.events.v1.TransactionEvent.TableSchema.json")
  }
}

# ------------------------------------------------------------------------------
# BigQuery Storage Resources
# ------------------------------------------------------------------------------

resource "google_bigquery_dataset" "dataset" {
  dataset_id                  = replace(local.name_prefix, "-", "_")
  friendly_name               = "Platform Events Dataset"
  description                 = "Dataset for data platform ingested events"
  location                    = var.region
  default_table_expiration_ms = null

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }

  depends_on = [google_project_service.apis]
}

resource "google_bigquery_table" "tables" {
  for_each = var.event_topics

  dataset_id          = google_bigquery_dataset.dataset.dataset_id
  table_id            = replace("${local.name_prefix}-${each.key}-table", "-", "_")
  description         = "Storage table for Pub/Sub topic: ${each.key}"
  deletion_protection = false

  schema = local.bigquery_schemas[each.key]

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }
}

# ------------------------------------------------------------------------------
# IAM Roles for Pub/Sub Service Agent
# ------------------------------------------------------------------------------

resource "google_bigquery_dataset_iam_member" "pubsub_bq_editor" {
  dataset_id = google_bigquery_dataset.dataset.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${local.pubsub_service_account}"
}

resource "google_bigquery_dataset_iam_member" "pubsub_bq_metadata" {
  dataset_id = google_bigquery_dataset.dataset.dataset_id
  role       = "roles/bigquery.metadataViewer"
  member     = "serviceAccount:${local.pubsub_service_account}"
}

# ------------------------------------------------------------------------------
# Pub/Sub BigQuery Subscriptions
# ------------------------------------------------------------------------------

resource "google_pubsub_subscription" "bigquery_subscriptions" {
  for_each = var.event_topics

  name  = "${local.name_prefix}-${each.key}-bq-sub"
  topic = google_pubsub_topic.topics[each.key].id

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.dataset.dataset_id}.${google_bigquery_table.tables[each.key].table_id}"
    use_topic_schema    = true
    drop_unknown_fields = true
  }

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }

  # Ensure permissions are set and table is created before creating subscription
  depends_on = [
    google_bigquery_table.tables,
    google_bigquery_dataset_iam_member.pubsub_bq_editor,
    google_bigquery_dataset_iam_member.pubsub_bq_metadata
  ]
}

# ------------------------------------------------------------------------------
# Cloud Run Ingestion Service Resources
# ------------------------------------------------------------------------------

# Dedicated Service Account for the Cloud Run ingestion service
resource "google_service_account" "ingestion_runner" {
  account_id   = "${local.name_prefix}-runner-sa"
  display_name = "Ingestion Service Runner Service Account"
  depends_on   = [google_project_service.apis]
}

# Grant Service Account publisher access to all Pub/Sub topics
resource "google_project_iam_member" "publisher" {
  project = var.project_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.ingestion_runner.email}"
}

# The Cloud Run Service itself
resource "google_cloud_run_v2_service" "ingestion" {
  name     = "${local.name_prefix}-ingestion"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.ingestion_runner.email

    containers {
      image = "gcr.io/${var.project_id}/ingestion-service:${var.image_tag}"

      ports {
        container_port = 8080
      }

      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }
      env {
        name  = "INGESTION_API_TOKEN"
        value = var.ingestion_api_token
      }
      env {
        name  = "PUBSUB_TOPIC_LOGIN"
        value = google_pubsub_topic.topics["login"].name
      }
      env {
        name  = "PUBSUB_TOPIC_LEVEL"
        value = google_pubsub_topic.topics["level"].name
      }
      env {
        name  = "PUBSUB_TOPIC_TRANSACTION"
        value = google_pubsub_topic.topics["transaction"].name
      }
    }
  }

  depends_on = [
    google_project_service.apis,
    google_pubsub_topic.topics
  ]
}

# Allow public unauthenticated invocations for external API token ingestion
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  name     = google_cloud_run_v2_service.ingestion.name
  location = google_cloud_run_v2_service.ingestion.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}
