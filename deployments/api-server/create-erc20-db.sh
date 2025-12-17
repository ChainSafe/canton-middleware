#!/bin/sh
# =============================================================================
# Creates the erc20_api database and runs the schema
# This script is executed by PostgreSQL's docker-entrypoint-initdb.d
# =============================================================================

set -e

# Create the erc20_api database
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -c "CREATE DATABASE erc20_api;"

# Run the schema against erc20_api
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "erc20_api" -f /docker-entrypoint-initdb.d/03-api-server-schema.sql

