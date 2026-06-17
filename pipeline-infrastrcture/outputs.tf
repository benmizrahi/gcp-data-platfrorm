output "pubsub_schemas" {
  description = "Map of schema keys to their schema IDs and names"
  value = {
    for k, v in google_pubsub_schema.schemas : k => {
      id   = v.id
      name = v.name
    }
  }
}

output "pubsub_topics" {
  description = "Map of topic keys to their topic names and IDs"
  value = {
    for k, v in google_pubsub_topic.topics : k => {
      id   = v.id
      name = v.name
    }
  }
}

output "schema_encoding" {
  description = "The message encoding configuration for the topics"
  value       = var.schema_encoding
}

output "bigquery_dataset" {
  description = "The ID of the BigQuery dataset"
  value       = google_bigquery_dataset.dataset.dataset_id
}

output "bigquery_tables" {
  description = "Map of table keys to their BigQuery table IDs"
  value = {
    for k, v in google_bigquery_table.tables : k => {
      id       = v.id
      table_id = v.table_id
    }
  }
}

output "bigquery_subscriptions" {
  description = "Map of subscription keys to their BigQuery subscription IDs"
  value = {
    for k, v in google_pubsub_subscription.bigquery_subscriptions : k => {
      id   = v.id
      name = v.name
    }
  }
}

output "ingestion_service_url" {
  description = "The public URL of the Cloud Run HTTP ingestion service"
  value       = google_cloud_run_v2_service.ingestion.uri
}

