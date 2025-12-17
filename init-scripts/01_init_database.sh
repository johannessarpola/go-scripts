#!/bin/bash
# Test script to verify multi-database setup is working correctly

set -e

echo "========================================"
echo "Testing Multi-Database Setup"
echo "========================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if docker compose is running
echo "1. Checking if database is running..."
if ! docker compose ps | grep -q "Up"; then
    echo -e "${RED}✗ Database is not running${NC}"
    echo "Please run: docker compose up -d"
    exit 1
fi
echo -e "${GREEN}✓ Database is running${NC}"
echo ""

# Wait for database to be ready
echo "2. Waiting for database to be ready..."
timeout=30
counter=0
while ! docker compose exec -T db pg_isready -U postgres > /dev/null 2>&1; do
    counter=$((counter + 1))
    if [ $counter -gt $timeout ]; then
        echo -e "${RED}✗ Database did not become ready in time${NC}"
        exit 1
    fi
    sleep 1
done
echo -e "${GREEN}✓ Database is ready${NC}"
echo ""

# List all databases
echo "3. Listing created databases..."
databases=$(docker compose exec -T db psql -U postgres -d postgres -t -c "SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres' ORDER BY datname;")

if [ -z "$databases" ]; then
    echo -e "${YELLOW}⚠ No custom databases found${NC}"
    echo "Did you place .sql files in data-dump/ directory?"
else
    echo -e "${GREEN}✓ Found the following databases:${NC}"
    echo "$databases" | sed 's/^/  - /'
fi
echo ""

# Test each database
echo "4. Testing database connections and table counts..."
for db in $databases; do
    db_name=$(echo $db | tr -d ' ')
    if [ -n "$db_name" ]; then
        echo "Testing: $db_name"
        
        # Try to connect and count tables
        table_count=$(docker compose exec -T db psql -U postgres -d "$db_name" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null || echo "0")
        table_count=$(echo $table_count | tr -d ' ')
        
        if [ "$table_count" -gt 0 ]; then
            echo -e "  ${GREEN}✓ Connected successfully - $table_count tables found${NC}"
            
            # List table names
            tables=$(docker compose exec -T db psql -U postgres -d "$db_name" -t -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;" 2>/dev/null)
            if [ -n "$tables" ]; then
                echo "  Tables:"
                echo "$tables" | sed 's/^/    - /'
            fi
        else
            echo -e "  ${YELLOW}⚠ Connected but no tables found${NC}"
        fi
        echo ""
    fi
done

# Show port mapping
echo "5. Port information..."
port=$(docker compose port db 5432 2>/dev/null || echo "Not exposed")
if [ "$port" != "Not exposed" ]; then
    echo -e "${GREEN}✓ Database exposed on: $port${NC}"
else
    echo -e "${YELLOW}⚠ Database port not exposed${NC}"
fi
echo ""

echo "========================================"
echo -e "${GREEN}Test Complete!${NC}"
echo "========================================"
echo ""
echo "Connect to a database:"
echo "  make psql DB=<database_name>"
echo ""
echo "Or directly:"
echo "  docker compose exec db psql -U postgres -d <database_name>"