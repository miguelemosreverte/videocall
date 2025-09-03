terraform {
  required_version = ">= 1.0"
  
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Variables
variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Cloud Run service name"
  type        = string
  default     = "websocket-hello-world"
}

# Provider configuration
provider "google" {
  project = var.project_id
  region  = var.region
}

# Enable required APIs
resource "google_project_service" "cloud_run_api" {
  service = "run.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "container_registry_api" {
  service = "containerregistry.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "cloud_build_api" {
  service = "cloudbuild.googleapis.com"
  disable_on_destroy = false
}

# Artifact Registry for container images
resource "google_artifact_registry_repository" "websocket_repo" {
  location      = var.region
  repository_id = "websocket-images"
  description   = "Docker repository for WebSocket server images"
  format        = "DOCKER"
  
  depends_on = [google_project_service.container_registry_api]
}

# Cloud Run service
resource "google_cloud_run_service" "websocket_service" {
  name     = var.service_name
  location = var.region

  template {
    spec {
      # Allow 1000 concurrent requests per container
      container_concurrency = 1000
      
      # Timeout for requests (maximum for Cloud Run)
      timeout_seconds = 3600
      
      containers {
        # Image URL - will be updated after building
        image = "${var.region}-docker.pkg.dev/${var.project_id}/websocket-images/websocket-server:latest"
        
        # Resources
        resources {
          limits = {
            cpu    = "2"
            memory = "2Gi"
          }
        }
        
        # Environment variables
        env {
          name  = "PORT"
          value = "8080"
        }
        
        # Health check
        startup_probe {
          http_get {
            path = "/health"
            port = 8080
          }
          initial_delay_seconds = 0
          timeout_seconds       = 1
          period_seconds        = 3
          failure_threshold     = 1
        }
      }
    }
    
    metadata {
      annotations = {
        # Use the maximum number of instances
        "autoscaling.knative.dev/maxScale"      = "100"
        "autoscaling.knative.dev/minScale"      = "0"
        # CPU is always allocated (needed for WebSockets)
        "run.googleapis.com/cpu-throttling"     = "false"
        # Use HTTP/2 for better WebSocket support
        "run.googleapis.com/h2c"                = "true"
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }

  depends_on = [
    google_project_service.cloud_run_api,
    google_artifact_registry_repository.websocket_repo
  ]
}

# IAM policy to allow public access
resource "google_cloud_run_service_iam_member" "public_access" {
  service  = google_cloud_run_service.websocket_service.name
  location = google_cloud_run_service.websocket_service.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Outputs
output "service_url" {
  value       = google_cloud_run_service.websocket_service.status[0].url
  description = "URL of the deployed WebSocket service"
}

output "container_image_url" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/websocket-images/websocket-server:latest"
  description = "Container image URL for updates"
}