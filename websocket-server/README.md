# WebSocket Hello World Server for Google Cloud Run

A high-performance WebSocket server written in Go, deployable to Google Cloud Run with Terraform.

## Features

- ✅ Written in Go for high performance
- ✅ Handles thousands of concurrent connections
- ✅ Automatic reconnection support
- ✅ Health checks for Cloud Run
- ✅ Infrastructure as Code with Terraform
- ✅ Simple web interface for testing
- ✅ JSON message format
- ✅ Broadcast to all connected clients

## Prerequisites

1. **Google Cloud Account** with billing enabled
2. **Tools installed:**
   ```bash
   # Check if installed
   gcloud --version
   terraform --version
   docker --version
   go version  # Optional, only for local testing
   ```

3. **Install missing tools:**
   ```bash
   # Install gcloud CLI
   # Visit: https://cloud.google.com/sdk/docs/install
   
   # Install Terraform
   brew install terraform  # macOS
   
   # Install Docker Desktop
   # Visit: https://www.docker.com/products/docker-desktop
   ```

## Quick Deploy

### 1. First-time Setup

```bash
# Login to Google Cloud
gcloud auth login
gcloud auth application-default login

# Create a new project (optional)
PROJECT_ID="websocket-demo-$(date +%s)"
gcloud projects create $PROJECT_ID
gcloud config set project $PROJECT_ID

# Enable billing (required)
echo "⚠️ Enable billing at: https://console.cloud.google.com/billing"
```

### 2. Deploy

```bash
# Clone and navigate to the directory
cd websocket-server

# Run the deployment script
./deploy.sh YOUR-PROJECT-ID us-central1
```

The script will:
1. Set up Terraform infrastructure
2. Build the Docker container
3. Push to Google Artifact Registry
4. Deploy to Cloud Run
5. Return the public URL

### 3. Test

Open the provided URL in your browser or use `wscat`:

```bash
# Install wscat
npm install -g wscat

# Connect to WebSocket
wscat -c wss://YOUR-SERVICE-URL.run.app/ws

# Send a message
> {"message": "Hello, World!"}
```

## Manual Deployment

### Step 1: Configure Terraform

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your project ID

terraform init
terraform plan
terraform apply
```

### Step 2: Build and Push Docker Image

```bash
# Configure Docker
gcloud auth configure-docker us-central1-docker.pkg.dev

# Build
docker build -t websocket-server .

# Tag
docker tag websocket-server:latest \
  us-central1-docker.pkg.dev/YOUR-PROJECT/websocket-images/websocket-server:latest

# Push
docker push us-central1-docker.pkg.dev/YOUR-PROJECT/websocket-images/websocket-server:latest
```

### Step 3: Deploy to Cloud Run

```bash
gcloud run deploy websocket-hello-world \
  --image us-central1-docker.pkg.dev/YOUR-PROJECT/websocket-images/websocket-server:latest \
  --region us-central1 \
  --allow-unauthenticated \
  --max-instances 100 \
  --memory 2Gi \
  --cpu 2
```

## Local Development

```bash
# Install dependencies
go mod download

# Run locally
go run main.go

# Visit http://localhost:8080
```

## Architecture

```
┌─────────────┐      WebSocket       ┌──────────────┐
│   Browser   │◄────────────────────►│  Cloud Run   │
│  Client 1   │                      │              │
└─────────────┘                      │  Go Server   │
                                     │              │
┌─────────────┐                      │   - Hub      │
│   Browser   │◄────────────────────►│   - Clients  │
│  Client 2   │                      │   - Broadcast│
└─────────────┘                      └──────────────┘
```

## Message Format

### Client → Server
```json
{
  "message": "Your message here"
}
```

### Server → Client
```json
{
  "type": "echo",
  "original": {"message": "Your message"},
  "server_time": "2024-01-20T10:30:00Z"
}
```

## Performance

- **Concurrent connections**: 1000 per container
- **Max instances**: 100 (100,000 total connections)
- **Memory**: 2GB per instance
- **CPU**: 2 vCPUs per instance
- **Timeout**: 1 hour (Cloud Run maximum)

## Cost Estimation

With Google Cloud Run:
- **Free tier**: 2 million requests/month
- **After free tier**: ~$0.40 per million requests
- **CPU**: $0.000024 per vCPU-second
- **Memory**: $0.0000025 per GB-second
- **Estimated monthly cost**: $0-50 for moderate usage

## Monitoring

View logs and metrics:
```bash
# View logs
gcloud logging read "resource.type=cloud_run_revision" --limit 50

# View metrics in Cloud Console
echo "https://console.cloud.google.com/run/detail/us-central1/websocket-hello-world/metrics"
```

## Clean Up

Remove all resources to avoid charges:
```bash
cd terraform
terraform destroy -auto-approve

# Or delete the entire project
gcloud projects delete YOUR-PROJECT-ID
```

## Next Steps for Video Streaming

This Hello World server can be extended for video event streaming:

1. **Increase buffer sizes** for video data
2. **Add binary message support** for efficient video chunks
3. **Implement rooms** for peer-to-peer connections
4. **Add Redis** for multi-instance state sharing
5. **Use Protocol Buffers** instead of JSON

## Troubleshooting

### "Permission denied" errors
```bash
gcloud projects add-iam-policy-binding YOUR-PROJECT \
  --member="user:YOUR-EMAIL" \
  --role="roles/owner"
```

### Docker push fails
```bash
gcloud auth configure-docker us-central1-docker.pkg.dev
```

### WebSocket connection fails
- Ensure you're using `wss://` not `ws://` for HTTPS
- Check Cloud Run logs: `gcloud logging read`
- Verify the service is running: `gcloud run services list`

## Support

For issues or questions about the deployment, check:
- Cloud Run logs in Google Cloud Console
- Terraform state: `terraform show`
- Service status: `gcloud run services describe websocket-hello-world`