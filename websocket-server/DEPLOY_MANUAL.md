# Manual Deployment Guide for videocall-signalling

## Project Configuration
- **Project ID**: `videocall-signalling`
- **Region**: `europe-west1` (Belgium - optimal for Europe/Russia)
- **Alternative**: `europe-north1` (Finland - closer to Russia)

## Step 1: Install Google Cloud CLI

### macOS
```bash
# Download and install
curl https://sdk.cloud.google.com | bash

# Restart shell
exec -l $SHELL

# Initialize
gcloud init
```

Or using Homebrew:
```bash
brew install --cask google-cloud-sdk
```

## Step 2: Authenticate and Set Project

```bash
# Login to Google Cloud
gcloud auth login

# Set your project
gcloud config set project videocall-signalling

# Set default region
gcloud config set run/region europe-west1

# Enable necessary APIs
gcloud services enable run.googleapis.com
gcloud services enable artifactregistry.googleapis.com
gcloud services enable cloudbuild.googleapis.com
```

## Step 3: Create Artifact Registry Repository

```bash
# Create repository for Docker images
gcloud artifacts repositories create websocket-images \
    --repository-format=docker \
    --location=europe-west1 \
    --description="WebSocket server images"

# Configure Docker authentication
gcloud auth configure-docker europe-west1-docker.pkg.dev
```

## Step 4: Build and Push Docker Image

```bash
# Navigate to websocket-server directory
cd /Users/miguel_lemos/Desktop/videocall/websocket-server

# Build the Docker image
docker build -t websocket-server .

# Tag for Artifact Registry
docker tag websocket-server:latest \
    europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest

# Push to Artifact Registry
docker push europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest
```

## Step 5: Deploy to Cloud Run

```bash
# Deploy the service
gcloud run deploy websocket-hello-world \
    --image europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest \
    --region europe-west1 \
    --platform managed \
    --allow-unauthenticated \
    --max-instances 100 \
    --min-instances 0 \
    --memory 2Gi \
    --cpu 2 \
    --timeout 3600 \
    --concurrency 1000 \
    --cpu-throttling=false \
    --port 8080

# The command will output your service URL
```

## Step 6: Test Your Deployment

Your service will be available at:
```
https://websocket-hello-world-[HASH]-ew.a.run.app
```

Test it:
1. Open the URL in your browser
2. Or test with curl:
```bash
# Get your service URL
SERVICE_URL=$(gcloud run services describe websocket-hello-world --region europe-west1 --format 'value(status.url)')
echo "Service URL: $SERVICE_URL"

# Test the health endpoint
curl $SERVICE_URL/health
```

## Alternative: Using Cloud Shell (No Installation Required)

1. Go to: https://console.cloud.google.com/cloudshell/editor
2. Upload the websocket-server folder
3. Run these commands in Cloud Shell:

```bash
# Everything is pre-installed in Cloud Shell
cd websocket-server

# Build and deploy
docker build -t websocket-server .
docker tag websocket-server:latest \
    europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest
docker push europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest

gcloud run deploy websocket-hello-world \
    --image europe-west1-docker.pkg.dev/videocall-signalling/websocket-images/websocket-server:latest \
    --region europe-west1 \
    --allow-unauthenticated
```

## Using Terraform (Optional)

If you prefer Infrastructure as Code:

```bash
cd terraform

# Create terraform.tfvars
cat > terraform.tfvars <<EOF
project_id = "videocall-signalling"
region = "europe-west1"
service_name = "websocket-hello-world"
EOF

# Initialize and apply
terraform init
terraform apply
```

## Region Comparison for Your Use Case

| Region | Location | Latency to Russia | Latency to W.Europe | Best For |
|--------|----------|-------------------|---------------------|----------|
| europe-west1 | Belgium | ~40ms | ~10ms | Balanced |
| europe-north1 | Finland | ~20ms | ~25ms | Closer to Russia |
| europe-central2 | Poland | ~30ms | ~20ms | Good middle ground |

Recommendation: **europe-north1 (Finland)** might be best for minimizing latency to Russia.

## Monitor Performance

```bash
# View logs
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=websocket-hello-world" \
    --limit 50 \
    --format json

# Check metrics
echo "Visit: https://console.cloud.google.com/run/detail/europe-west1/websocket-hello-world/metrics?project=videocall-signalling"
```

## Estimated Costs

For your video call use case:
- **WebSocket connections**: Free tier includes 2M requests
- **CPU time**: ~$5-10/month for moderate usage
- **Bandwidth**: EU to Russia egress: ~$0.12/GB
- **Total estimate**: $10-30/month for regular video calls

## Clean Up (When Needed)

```bash
# Delete the Cloud Run service
gcloud run services delete websocket-hello-world --region europe-west1

# Delete the Artifact Registry repository
gcloud artifacts repositories delete websocket-images --location=europe-west1

# Or use Terraform
cd terraform
terraform destroy
```

## Next Steps for Video Integration

Once this Hello World works, we'll:
1. Modify the server to handle video motion events
2. Implement binary frames for efficiency  
3. Add peer-to-peer room management
4. Optimize for your specific EU-Russia route