#!/bin/bash
set -e

DUMPS_DIR="/docker-entrypoint-initdb.d/dumps"
CONFIG_FILE="/docker-entrypoint-initdb.d/config.yml"

echo "Scanning for SQL dump files in $DUMPS_DIR..."

# Read extensions from YAML config file
read_extensions() {
    if [ ! -f "$CONFIG_FILE" ]; then
        echo "Warning: $CONFIG_FILE not found, skipping extension installation"
        return
    fi
    
    # Parse YAML and extract extension names (lines starting with "  - ")
    grep "^  - " "$CONFIG_FILE" | sed 's/^  - //' | tr -d '\r'
}

# Loop through all .sql files in the dumps directory
for dump_file in "$DUMPS_DIR"/*.sql; do
    # Skip if no files match
    [ -e "$dump_file" ] || continue
    
    # Extract database name from filename (remove path and .sql extension)
    db_name=$(basename "$dump_file" .sql)
    
    echo "Processing: $db_name"
    
    # Create database
    echo "  Creating database: $db_name"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        CREATE DATABASE "$db_name";
EOSQL
    
    # Pre-create extensions from config file
    extensions=$(read_extensions)
    if [ -n "$extensions" ]; then
        echo "  Installing extensions from config..."
        while IFS= read -r ext; do
            [ -z "$ext" ] && continue
            echo "    - $ext"
            # Handle extensions with hyphens (need quotes)
            if [[ "$ext" == *"-"* ]]; then
                psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db_name" <<-EOSQL
                    CREATE EXTENSION IF NOT EXISTS "$ext" WITH SCHEMA public;
EOSQL
            else
                psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db_name" <<-EOSQL
                    CREATE EXTENSION IF NOT EXISTS $ext WITH SCHEMA public;
EOSQL
            fi
        done <<< "$extensions"
    fi
    
    # Import dump - extensions already exist, so CREATE EXTENSION IF NOT EXISTS will be no-ops
    echo "  Importing dump: $dump_file"
    psql --username "$POSTGRES_USER" --dbname "$db_name" -v ON_ERROR_STOP=1 < "$dump_file"
    
    echo "  ✓ Completed: $db_name"
done

echo "All dumps imported successfully!"