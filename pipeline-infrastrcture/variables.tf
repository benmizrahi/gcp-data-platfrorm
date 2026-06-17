variable "project_id" {
  type        = string
  description = "The GCP project ID where resources will be provisioned"
}

variable "region" {
  type        = string
  description = "The primary GCP region for resources"
  default     = "us-central1"
}

variable "environment" {
  type        = string
  description = "The deployment environment (e.g., dev, staging, prod)"
  default     = "dev"
}

variable "schema_encoding" {
  type        = string
  description = "The encoding of the messages validated against the schema (JSON or BINARY)"
  default     = "JSON"
  validation {
    condition     = contains(["JSON", "BINARY"], var.schema_encoding)
    error_message = "The schema_encoding must be either 'JSON' or 'BINARY'."
  }
}

variable "event_topics" {
  type = map(object({
    message_type = string
  }))
  description = "Map of Pub/Sub topics to create with their associated Protobuf message types"
  default = {
    "login" = {
      message_type = "platform.events.v1.LoginEvent"
    }
    "level" = {
      message_type = "platform.events.v1.LevelEvent"
    }
    "transaction" = {
      message_type = "platform.events.v1.TransactionEvent"
    }
  }
}

variable "ingestion_api_token" {
  type        = string
  description = "The secure API token used to authenticate client ingestion requests"
  sensitive   = true
  default     = "dev-secure-token-12345"
}

variable "image_tag" {
  type        = string
  description = "The tag of the ingestion-service docker image to deploy"
  default     = "latest"
}

