# Terraform configuration for video conference infrastructure
# This sets up a domain with automatic SSL certificate from Let's Encrypt

terraform {
  required_version = ">= 1.0"
  
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
    acme = {
      source  = "vancluever/acme"
      version = "~> 2.0"
    }
  }
}

# Variables
variable "cloudflare_api_token" {
  description = "Cloudflare API token for DNS management"
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for your domain"
  type        = string
}

variable "domain_name" {
  description = "Domain name for the conference server (e.g., conference.example.com)"
  type        = string
  default     = "conference.yourdomain.com"
}

variable "server_ip" {
  description = "IP address of your VPS server"
  type        = string
  default     = "194.87.103.57"
}

variable "acme_email" {
  description = "Email address for Let's Encrypt registration"
  type        = string
}

# Providers
provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "acme" {
  server_url = "https://acme-v02.api.letsencrypt.org/directory"
}

# Create A record pointing to VPS
resource "cloudflare_record" "conference" {
  zone_id = var.cloudflare_zone_id
  name    = var.domain_name
  value   = var.server_ip
  type    = "A"
  ttl     = 120
  proxied = false # Don't proxy through Cloudflare for WebSocket
}

# Create ACME registration
resource "acme_registration" "conference" {
  account_key_pem = tls_private_key.conference.private_key_pem
  email_address   = var.acme_email
}

# Generate private key for ACME
resource "tls_private_key" "conference" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

# Request certificate from Let's Encrypt
resource "acme_certificate" "conference" {
  account_key_pem           = acme_registration.conference.account_key_pem
  common_name               = var.domain_name
  subject_alternative_names = [var.domain_name]

  dns_challenge {
    provider = "cloudflare"
    
    config = {
      CF_API_TOKEN = var.cloudflare_api_token
      CF_ZONE_ID   = var.cloudflare_zone_id
    }
  }
}

# Output the certificate details
output "certificate_pem" {
  value     = acme_certificate.conference.certificate_pem
  sensitive = true
}

output "private_key_pem" {
  value     = acme_certificate.conference.private_key_pem
  sensitive = true
}

output "full_chain_pem" {
  value     = "${acme_certificate.conference.certificate_pem}${acme_certificate.conference.issuer_pem}"
  sensitive = true
}

output "domain_url" {
  value = "https://${var.domain_name}"
}

output "websocket_url" {
  value = "wss://${var.domain_name}:3001/ws"
}