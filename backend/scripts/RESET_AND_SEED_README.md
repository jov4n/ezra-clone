# Database Reset and Seed Script

This script completely resets your Neo4j database and re-seeds it with a fresh enhanced schema.

## ⚠️ WARNING

**This script will DELETE ALL DATA from your Neo4j database!** This action cannot be undone. Make sure you have backups if you need to preserve any data.

## What It Does

1. **Deletes All Data** - Removes all nodes and relationships from Neo4j
2. **Drops Constraints & Indexes** - Removes all existing constraints and indexes
3. **Creates Enhanced Schema** - Sets up constraints and indexes for the enhanced schema
4. **Creates Agent** - Creates the default agent (Ezra by default)
5. **Creates Agent Identity** - Sets up the agent's personality and capabilities
6. **Creates Memory Blocks** - Sets up default memory blocks (identity, instructions, persona)
7. **Creates Initial Topics** - Creates initial topic structure
8. **Links Topics** - Creates relationships between topics
9. **Marks Migration** - Marks the enhanced schema migration as applied

## Usage

### Basic Usage

```bash
cd backend/scripts
go run reset_and_seed.go
```

The script will prompt you for confirmation before proceeding.

### Skip Confirmation

If you want to skip the confirmation prompt (useful for automation):

```bash
go run reset_and_seed.go -y
```

### Custom Agent ID

To create an agent with a different ID:

```bash
go run reset_and_seed.go -agent-id "MyAgent"
```

### Build and Run

You can also build the script and run it directly:

```bash
# Build
cd backend/scripts
go build -o reset_and_seed.exe reset_and_seed.go

# Run (Windows)
.\reset_and_seed.exe

# Run with flags
.\reset_and_seed.exe -y -agent-id "Ezra"
```

## Prerequisites

- Neo4j database running and accessible
- Configuration file (`.env` or environment variables) with Neo4j credentials:
  - `NEO4J_URI`
  - `NEO4J_USER`
  - `NEO4J_PASSWORD`

## What Gets Created

### Agent
- Default agent with ID "Ezra" (or custom ID if specified)
- Agent identity with personality and capabilities

### Memory Blocks
- **identity**: Agent's core identity and traits
- **instructions**: Operating instructions for the agent
- **persona**: Personality guidelines

### Topics
- General
- Technology
- Entertainment
- Personal
- Preferences
- Life Events

### Schema
- All constraints for data integrity
- All indexes for performance
- Full-text indexes (if supported by Neo4j version)
- Enhanced schema migration marked as applied

## Example Output

```
INFO    Starting database reset and seed...
⚠️  WARNING: This will DELETE ALL DATA from Neo4j!
This action cannot be undone.
Are you sure you want to continue? (yes/no): yes
INFO    Step 1: Deleting all data from Neo4j...
INFO    All nodes and relationships deleted
INFO    Step 2: Dropping all constraints and indexes...
INFO    Step 3: Creating constraints...
INFO    Step 4: Creating indexes...
INFO    Step 5: Creating agent
INFO    Step 6: Creating agent identity
INFO    Step 7: Creating default memory blocks
INFO    Step 8: Creating initial topics
INFO    Step 9: Linking topics
INFO    Step 10: Marking migration as applied
INFO    ✅ Database reset and seed completed successfully!
INFO    The database is now fresh and ready to use!
```

## Safety Features

- **Confirmation Prompt**: By default, requires explicit "yes" confirmation
- **Logging**: All steps are logged for visibility
- **Error Handling**: Fails gracefully with clear error messages
- **Idempotent**: Safe to run multiple times (though it will delete everything each time)

## Use Cases

- **Development**: Start fresh during development
- **Testing**: Reset database before running tests
- **Migration**: After schema changes, reset and re-seed
- **Cleanup**: Remove all test data and start clean

## Notes

- The script uses the same configuration system as the rest of the application
- Full-text indexes may not be created if your Neo4j version doesn't support them (this is okay)
- Some warnings about existing constraints/indexes are normal and can be ignored
- The script is designed to be safe but destructive - use with caution!

## Troubleshooting

### Connection Errors
- Verify Neo4j is running
- Check your `.env` file or environment variables
- Ensure credentials are correct

### Permission Errors
- Ensure the Neo4j user has permissions to delete data and create constraints/indexes
- Check Neo4j configuration for write access

### Constraint/Index Errors
- Some warnings are normal (e.g., "already exists")
- If critical errors occur, check Neo4j logs

