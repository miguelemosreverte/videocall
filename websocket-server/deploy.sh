#!/bin/bash
set -e

echo "🚀 WebSocket Server Deployment Script"
echo "===================================="

# Check if required tools are installed
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
}

check_tool "gcloud"
check_tool "terraform"
check_tool "docker"

# Get project ID
if [ -z "$1" ]; then
    echo "Usage: ./deploy.sh <PROJECT_ID> [REGION]"
    echo "Example: ./deploy.sh my-gcp-project us-central1"
    exit 1
fi

PROJECT_ID=$1
REGION=${2:-europe-west1}
SERVICE_NAME="websocket-hello-world"

echo "📋 Configuration:"
echo "  Project ID: $PROJECT_ID"
echo "  Region: $REGION"
echo "  Service: $SERVICE_NAME"
echo ""

# Set the project
echo "1️⃣ Setting GCP project..."
gcloud config set project $PROJECT_ID

# Authenticate Docker with Google Artifact Registry
echo "2️⃣ Configuring Docker authentication..."
gcloud auth configure-docker ${REGION}-docker.pkg.dev

# Initialize Terraform
echo "3️⃣ Initializing Terraform..."
cd terraform
terraform init

# Create terraform.tfvars if it doesn't exist
if [ ! -f terraform.tfvars ]; then
    echo "Creating terraform.tfvars..."
    cat > terraform.tfvars <<EOF
project_id = "${PROJECT_ID}"
region = "${REGION}"
service_name = "${SERVICE_NAME}"
EOF
fi

# Apply Terraform to create infrastructure
echo "4️⃣ Creating infrastructure with Terraform..."
terraform apply -auto-approve

# Build and push Docker image
echo "5️⃣ Building Docker image..."
cd ..
IMAGE_URL="${REGION}-docker.pkg.dev/${PROJECT_ID}/websocket-images/websocket-server:latest"

# Build the image
docker build -t websocket-server .

# Tag for Artifact Registry
docker tag websocket-server:latest ${IMAGE_URL}

# Push to Artifact Registry
echo "6️⃣ Pushing Docker image to Artifact Registry..."
docker push ${IMAGE_URL}

# Deploy to Cloud Run (update the service with new image)
echo "7️⃣ Deploying to Cloud Run..."
gcloud run deploy ${SERVICE_NAME} \
    --image ${IMAGE_URL} \
    --region ${REGION} \
    --platform managed \
    --allow-unauthenticated \
    --max-instances 100 \
    --min-instances 0 \
    --memory 2Gi \
    --cpu 2 \
    --timeout 3600 \
    --concurrency 1000

# Get the service URL
SERVICE_URL=$(gcloud run services describe ${SERVICE_NAME} --region ${REGION} --format 'value(status.url)')

echo ""
echo "✅ Deployment complete!"
echo "===================================="
echo "🌐 WebSocket Server URL: ${SERVICE_URL}"
echo "🔧 Test WebSocket endpoint: ${SERVICE_URL}/ws"
echo "🖥️ Web interface: ${SERVICE_URL}"
echo ""
echo "To test the WebSocket connection:"
echo "  1. Open ${SERVICE_URL} in your browser"
echo "  2. Or use wscat: wscat -c ${SERVICE_URL}/ws"
echo ""
echo "To delete all resources:"
echo "  cd terraform && terraform destroy"