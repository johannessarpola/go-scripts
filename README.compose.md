# Database Docker Compose Setup

A Docker Compose configuration for PostgreSQL with automatic data dump initialization, configurable ports, and container naming.

## Features

- **Automatic Data Loading**: SQL files in `data-dump/` are executed on first startup
- **Configurable Port**: Set a fixed port or use a random port via `.env`
- **Custom Container Name**: Override the default container name via `.env`
- **Health Checks**: Built-in health checking for the database
- **Data Persistence**: Database data persists in a named volume

## Quick Start

1. **Create your `.env` file**:
   ```bash
   cp .env.example .env
   ```

2. **Add your data dump**:
   Place your SQL files in the `data-dump/` directory. Files are executed in alphabetical order.

3. **Start the database**:
   ```bash
   docker compose up -d
   ```

4. **Check the assigned port** (if using random port):
   ```bash
   docker compose ps
   # or
   docker port $(docker compose ps -q db) 5432
   ```

## Configuration Options

### Port Configuration

**Fixed Port** (recommended for development):
```env
DB_PORT=5433
```

**Random Port** (useful for avoiding conflicts):
```env
DB_PORT=0
```
Or leave it unset - Docker will assign a random available port.

To find the assigned port:
```bash
docker compose port db 5432
# Output: 0.0.0.0:54321 (example)
```

### Container Name

```env
DB_CONTAINER_NAME=my_custom_db
```

Default: `my_database`

### Database Credentials

```env
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=mydb
```

## Data Dump Format

Place SQL files in `data-dump/`. Supported formats:
- `.sql` - Plain SQL files
- `.sql.gz` - Compressed SQL files
- `.sh` - Shell scripts

Files are executed in alphabetical order, so prefix with numbers:
```
data-dump/
├── 01-schema.sql
├── 02-data.sql
└── 03-indexes.sql
```

## Connecting to the Database

### From Host Machine

```bash
# If using fixed port (e.g., 5433)
psql -h localhost -p 5433 -U postgres -d mydb

# If using random port, first get the port
PORT=$(docker compose port db 5432 | cut -d: -f2)
psql -h localhost -p $PORT -U postgres -d mydb
```

### From Another Container

```bash
# Add to your docker-compose.yml:
depends_on:
  db:
    condition: service_healthy

# Connection string:
postgresql://postgres:password@db:5432/mydb
```

## Useful Commands

```bash
# Start services
docker compose up -d

# View logs
docker compose logs -f db

# Stop services
docker compose down

# Stop and remove volumes (fresh start)
docker compose down -v

# Execute SQL command
docker compose exec db psql -U postgres -d mydb -c "SELECT * FROM users;"

# Restore from a dump file
docker compose exec -T db psql -U postgres -d mydb < backup.sql

# Create a backup
docker compose exec db pg_dump -U postgres mydb > backup.sql
```

## Resetting the Database

To start fresh with a clean database:

```bash
# Stop and remove volumes
docker compose down -v

# Start again (will re-run initialization scripts)
docker compose up -d
```

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PORT` | `0` (random) | Host port to expose PostgreSQL |
| `DB_CONTAINER_NAME` | `my_database` | Container name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | `postgres` | Database password |
| `POSTGRES_DB` | `mydb` | Database name |

## Troubleshooting

**Port already in use**:
- Set `DB_PORT=0` in `.env` to use a random port
- Or change to a different fixed port

**Data not loading**:
- Check logs: `docker compose logs db`
- Ensure SQL files have correct syntax
- Files only run on first initialization (empty volume)
- To re-run: `docker compose down -v && docker compose up -d`

**Connection refused**:
- Wait for health check: `docker compose ps`
- Check the service is healthy before connecting