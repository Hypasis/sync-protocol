#!/bin/bash

# Hypasis Sync Protocol - Cloud Deployment Script
# This script helps deploy Hypasis in cloud mode

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}"
echo "╔═══════════════════════════════════════════════════╗"
echo "║   Hypasis Sync Protocol - Cloud Deployment       ║"
echo "╚═══════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${YELLOW}⚠️  .env file not found. Creating from .env.example...${NC}"
    cp .env.example .env
    echo -e "${RED}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "⚠️  IMPORTANT: Please edit .env and fill in:"
    echo "   - ETH_L1_RPC_URL (Ethereum mainnet RPC)"
    echo "   - POLYGON_RPC_1 (Polygon RPC)"
    echo "   - REDIS_PASSWORD (strong password)"
    echo "   - JWT_SECRET (random secret)"
    echo ""
    echo "Then run this script again."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${NC}"
    exit 1
fi

# Load environment variables
source .env

# Validate required variables
REQUIRED_VARS=("ETH_L1_RPC_URL" "INSTANCE_ID")
MISSING_VARS=()

for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        MISSING_VARS+=("$var")
    fi
done

if [ ${#MISSING_VARS[@]} -ne 0 ]; then
    echo -e "${RED}❌ Missing required environment variables:${NC}"
    for var in "${MISSING_VARS[@]}"; do
        echo "   - $var"
    done
    echo ""
    echo "Please edit .env and set these variables."
    exit 1
fi

echo -e "${GREEN}✅ Environment variables validated${NC}"
echo ""

# Function to check if Docker is installed
check_docker() {
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}❌ Docker is not installed${NC}"
        echo "Please install Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi
    echo -e "${GREEN}✅ Docker found${NC}"
}

# Function to check if docker-compose is installed
check_docker_compose() {
    if ! command -v docker-compose &> /dev/null; then
        echo -e "${RED}❌ docker-compose is not installed${NC}"
        echo "Please install docker-compose: https://docs.docker.com/compose/install/"
        exit 1
    fi
    echo -e "${GREEN}✅ docker-compose found${NC}"
}

# Check prerequisites
echo "🔍 Checking prerequisites..."
check_docker
check_docker_compose
echo ""

# Ask deployment mode
echo "📋 Select deployment mode:"
echo "  1) Single instance (testing)"
echo "  2) Multi-instance cluster (production)"
echo ""
read -p "Enter choice [1-2]: " DEPLOY_MODE

if [ "$DEPLOY_MODE" = "1" ]; then
    echo ""
    echo -e "${YELLOW}📦 Deploying single instance...${NC}"
    docker-compose up -d hypasis-1 redis

    echo ""
    echo -e "${GREEN}✅ Single instance deployed!${NC}"
    echo ""
    echo "🔗 Endpoints:"
    echo "   API:     http://localhost:8080"
    echo "   RPC:     http://localhost:8545"
    echo "   Metrics: http://localhost:9090/metrics"
    echo ""
    echo "📋 View logs:"
    echo "   docker logs -f hypasis-sync-1"
    echo ""
    echo "🛑 Stop:"
    echo "   docker-compose down"

elif [ "$DEPLOY_MODE" = "2" ]; then
    echo ""
    echo -e "${YELLOW}📦 Deploying multi-instance cluster...${NC}"
    docker-compose -f docker-compose.cloud.yaml up -d

    echo ""
    echo -e "${GREEN}✅ Multi-instance cluster deployed!${NC}"
    echo ""
    echo "🌐 Instances:"
    echo "   Instance 1: http://localhost:8080"
    echo "   Instance 2: http://localhost:8081"
    echo "   Instance 3: http://localhost:8082"
    echo ""
    echo "📊 Monitoring:"
    echo "   Prometheus: http://localhost:9099"
    echo "   Grafana:    http://localhost:3000 (admin/admin)"
    echo ""
    echo "📋 View logs:"
    echo "   docker-compose -f docker-compose.cloud.yaml logs -f"
    echo ""
    echo "🛑 Stop all:"
    echo "   docker-compose -f docker-compose.cloud.yaml down"
else
    echo -e "${RED}Invalid choice${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}⏳ Waiting for services to be healthy (30 seconds)...${NC}"
sleep 30

# Check health
echo ""
echo "🏥 Health check:"
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Hypasis is healthy${NC}"
else
    echo -e "${YELLOW}⚠️  Health check pending (may take a few minutes to fully start)${NC}"
fi

# Get enode URL (for production deployment)
echo ""
echo "📡 Bootnode Information:"
echo ""
echo "After the service is fully started, get your enode URL with:"
echo "   docker exec hypasis-sync-1 /app/hypasis-sync --help"
echo ""

# Print next steps
echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              🎉 Deployment Complete!              ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════╝${NC}"
echo ""
echo "📖 Next Steps:"
echo ""
echo "1. Check status:"
echo "   curl http://localhost:8080/api/v1/status"
echo ""
echo "2. View cluster status (multi-instance only):"
echo "   curl http://localhost:8080/api/v1/cluster"
echo ""
echo "3. Monitor sync progress:"
echo "   watch -n 5 'curl -s http://localhost:8080/api/v1/status | jq .'"
echo ""
echo "4. Get enode URLs for node operators:"
echo "   curl http://localhost:8080/api/v1/bootnodes"
echo ""
echo "📚 Documentation:"
echo "   - Deployment Guide: CLOUD_DEPLOYMENT.md"
echo "   - Node Operator Guide: NODE_OPERATOR_GUIDE.md"
echo ""
echo "💬 Support:"
echo "   - GitHub: https://github.com/hypasis/sync-protocol"
echo "   - Discord: https://discord.gg/hypasis"
echo ""
