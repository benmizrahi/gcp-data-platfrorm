# GCP Data Platform Orchestration Makefile
# Set PROJECT_ID to your target GCP Project (default is catchme-poc)
PROJECT_ID ?= catchme-poc
COMMIT_SHA := $(shell git log -1 --format="%H" 2>/dev/null || echo "latest")

.PHONY: all generate build deploy clean

all: generate build deploy

# Step 1: Run Buf code generation to compile protobufs and generate BigQuery schemas
generate:
	@echo "==> Generating schemas using Buf..."
	cd schema-managment && buf generate

# Step 2: Build and push the Ingestion Service docker image via Cloud Build
# Run from the workspace root context so the builder can see go.work, ingestion-service, and schema-managment
build: generate
	@echo "==> Submitting Cloud Build for ingestion-service..."
	gcloud builds submit --config ingestion-service/cloudbuild.yaml --project $(PROJECT_ID) --substitutions=COMMIT_SHA=$(COMMIT_SHA) .

# Step 3: Run Terraform apply to deploy the entire environment
deploy: build
	@echo "==> Provisioning infrastructure with Terraform..."
	cd pipeline-infrastrcture && terraform init && terraform apply -var="project_id=$(PROJECT_ID)" -var="image_tag=$(COMMIT_SHA)" -auto-approve


clean:
	@echo "==> Cleaning up generated artifacts..."
	rm -rf schema-managment/gen/go/*
	rm -rf schema-managment/gen/bq/*
