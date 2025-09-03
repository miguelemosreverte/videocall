#!/bin/bash

# WebP Conference Server - Docker Deployment Script
# Usage: ./deploy.sh [build|start|stop|restart|logs|status]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

# Check if Docker is installed
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    
    print_status "Docker and Docker Compose are installed"
}

# Check if .env file exists
check_env() {
    if [ ! -f .env ]; then
        print_warning ".env file not found. Creating from template..."
        cp .env.example .env
        print_warning "Please edit .env file with your configuration before deploying"
        exit 1
    fi
    print_status ".env file found"
}

# Build Docker image
build() {
    print_status "Building Docker image..."
    docker-compose build --no-cache
    print_status "Docker image built successfully"
}

# Start the server
start() {
    print_status "Starting WebP Conference Server..."
    docker-compose up -d
    print_status "Server started successfully"
    echo ""
    print_status "Server is running on:"
    
    # Read domain from .env
    if [ -f .env ]; then
        DOMAIN=$(grep DOMAIN .env | cut -d '=' -f2)
        USE_SSL=$(grep USE_SSL .env | cut -d '=' -f2)
        
        if [ "$USE_SSL" = "true" ]; then
            echo "  • https://$DOMAIN"
            echo "  • wss://$DOMAIN/ws"
        else
            echo "  • http://$DOMAIN"
            echo "  • ws://$DOMAIN/ws"
        fi
    fi
}

# Stop the server
stop() {
    print_status "Stopping WebP Conference Server..."
    docker-compose down
    print_status "Server stopped"
}

# Restart the server
restart() {
    stop
    start
}

# View logs
logs() {
    docker-compose logs -f
}

# Check server status
status() {
    if docker-compose ps | grep -q "Up"; then
        print_status "Server is running"
        docker-compose ps
    else
        print_warning "Server is not running"
        docker-compose ps
    fi
}

# Clean up Docker resources
clean() {
    print_warning "This will remove all containers, images, and volumes for this project"
    read -p "Are you sure? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker-compose down -v --rmi all
        print_status "Cleanup complete"
    else
        print_status "Cleanup cancelled"
    fi
}

# Main script
main() {
    echo "======================================="
    echo "   WebP Conference Server Deployment   "
    echo "======================================="
    echo ""
    
    check_docker
    
    case "$1" in
        build)
            check_env
            build
            ;;
        start)
            check_env
            start
            ;;
        stop)
            stop
            ;;
        restart)
            check_env
            restart
            ;;
        logs)
            logs
            ;;
        status)
            status
            ;;
        clean)
            clean
            ;;
        *)
            echo "Usage: $0 {build|start|stop|restart|logs|status|clean}"
            echo ""
            echo "Commands:"
            echo "  build    - Build Docker image"
            echo "  start    - Start the server"
            echo "  stop     - Stop the server"
            echo "  restart  - Restart the server"
            echo "  logs     - View server logs"
            echo "  status   - Check server status"
            echo "  clean    - Remove all Docker resources"
            exit 1
            ;;
    esac
}

main "$@"