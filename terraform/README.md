# Terraform Infrastructure for Video Conference

This Terraform configuration automates:
1. Domain setup with Cloudflare DNS
2. Let's Encrypt SSL certificate generation
3. Automatic certificate renewal

## Prerequisites

1. **Domain Name**: You need to own a domain (e.g., from Namecheap, GoDaddy, etc.)
2. **Cloudflare Account**: Free account at [cloudflare.com](https://cloudflare.com)
3. **Terraform**: Install from [terraform.io](https://terraform.io)

## Setup Instructions

### Step 1: Set up Cloudflare

1. Add your domain to Cloudflare (free plan is fine)
2. Update your domain's nameservers to Cloudflare's
3. Get your Zone ID from Cloudflare dashboard
4. Create API token:
   - Go to https://dash.cloudflare.com/profile/api-tokens
   - Create token with permission: `Zone:DNS:Edit` for your zone

### Step 2: Configure Terraform

1. Copy the example variables file:
```bash
cp terraform.tfvars.example terraform.tfvars
```

2. Edit `terraform.tfvars` with your values:
```hcl
cloudflare_api_token = "your-token"
cloudflare_zone_id   = "your-zone-id"
domain_name         = "conference.yourdomain.com"  # Your subdomain
server_ip           = "194.87.103.57"              # Your VPS IP
acme_email          = "your@email.com"
```

### Step 3: Deploy

```bash
# Initialize Terraform
terraform init

# Review the plan
terraform plan

# Apply the configuration (creates domain + certificate)
terraform apply

# Deploy certificates to VPS
chmod +x deploy-certificates.sh
./deploy-certificates.sh
```

### Step 4: Update your HTML

After deployment, update your `index.html` to use the new domain:

```javascript
const defaultUrl = 'wss://conference.yourdomain.com:3001/ws';
```

## What This Creates

- **DNS Record**: A record pointing `conference.yourdomain.com` to your VPS
- **SSL Certificate**: Valid Let's Encrypt certificate (auto-renews)
- **No more warnings**: Users can connect directly without certificate warnings

## Certificate Renewal

Let's Encrypt certificates expire every 90 days. To renew:

```bash
terraform apply
./deploy-certificates.sh
```

You can automate this with a cron job.

## Costs

- **Cloudflare**: Free
- **Let's Encrypt**: Free
- **Domain**: ~$10/year (varies by provider)

## Troubleshooting

If certificates fail to generate:
- Ensure your domain's nameservers point to Cloudflare
- Wait 5-10 minutes for DNS propagation
- Check Cloudflare API token permissions

## Alternative: Use Certbot Directly

If you prefer not to use Terraform, you can use certbot directly on the VPS:

```bash
# On the VPS
apt install certbot
certbot certonly --standalone -d conference.yourdomain.com
# Certificates will be in /etc/letsencrypt/live/conference.yourdomain.com/
```