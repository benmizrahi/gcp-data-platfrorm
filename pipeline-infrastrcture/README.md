# GCP Data Platform Ingestion Pipeline Infrastructure

This directory contains the Terraform module used to provision GCP Pub/Sub topics and their corresponding Protobuf schemas.

> [!IMPORTANT]
> This infrastructure **directly references and loads the existing schema definitions** from the `schema-managment` workspace folder. No schema files are duplicated! Any schema updates made in the `schema-managment` module are dynamically read and applied by Terraform.

## Architecture & Design Decisions

### 1. Polymorphic vs. Specific Topics
This infrastructure sets up two patterns of event ingestion:
*   **Unified Event Topic (`event-envelope`)**: A single topic containing the base `Event` envelope with common metadata, which uses a `oneof` field containing the individual concrete payloads (`LoginEvent`, `TransactionEvent`, `LevelEvent`). This maps directly to the unified route in the `ingestion-service`.
*   **Dedicated Event Topics (`login`, `level`, `transaction`)**: Individual topics for each distinct event type, providing flexibility if downstream consumers only require specialized event data.

### 2. Dynamic Inlining of Dependencies (No Code Duplication)
Google Cloud Pub/Sub schemas restrict the use of custom Protobuf `import` statements within the schema registry:
*   Standard imports like `google/protobuf/struct.proto` and `google/protobuf/timestamp.proto` are natively supported and do not need inlining.
*   Custom platform-specific imports (such as importing `login.proto` inside `event.proto`) are **not supported** by Pub/Sub.
*   **Solution**: In `main.tf`, we use Terraform's built-in `file()` and regex `replace()` functions to dynamically construct a flattened, self-contained `event.proto` schema on the fly. It extracts the core schema messages from `schema-managment/proto` and strips out syntactical package conflicts, producing a fully compliant polymorphic envelope without duplicate file maintenance.

## Codebase Structure

```
pipeline-infrastrcture/
├── main.tf            # Dynamically loads and flattens schemas from schema-managment workspace
├── variables.tf       # Defines project, region, environment, and topics
└── outputs.tf         # Outputs details of created schemas and topics
```

## Getting Started

### Prerequisite
Ensure you have configured your Google Cloud SDK credentials:
```bash
gcloud auth application-default login
```

### Configuration
Create a `terraform.tfvars` file with your GCP Project details:
```hcl
project_id      = "your-gcp-project-id"
region          = "us-central1"
environment     = "dev"
schema_encoding = "JSON"
```

### Deployment
Initialize and apply the Terraform configuration:
```bash
terraform init
terraform plan
terraform apply
```
