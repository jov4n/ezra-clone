# Neo4j Enhanced Schema Migration

This migration script enhances your Neo4j database schema with:

## Features Added

1. **Discord Entity Tracking**
   - Guild/Server nodes with relationships
   - Channel nodes linked to guilds
   - Role nodes with permissions
   - User-to-guild membership tracking

2. **User-to-User Relationships**
   - `MENTIONED` - tracks when users mention each other
   - `REPLIED_TO` - tracks reply patterns with response times
   - `SHARED_TOPIC` - tracks shared interests between users
   - `COLLABORATED` - tracks collaboration in conversations
   - `SIMILAR_TO` - similarity relationships based on shared interests

3. **Weighted & Temporal Relationships**
   - `INTERESTED_IN` now includes: strength, interaction_count, recency_score
   - `TOLD_ME` now includes: fact_count, trust_score, timestamps
   - `PARTICIPATED_IN` now includes: message_count, activity_score, timestamps

4. **Enhanced Message Threading**
   - `REPLIES_TO` relationships between messages
   - `MENTIONS` relationships from messages to users
   - `ABOUT_TOPIC` relationships from messages to topics

5. **Activity Patterns**
   - Activity pattern nodes tracking user behavior by day/hour
   - `ACTIVE_AT` relationships with frequency data

6. **Fact Relationships**
   - `SUPPORTS`, `CONTRADICTS`, `RELATED_TO` between facts
   - `VERIFIED_BY` and `CHALLENGED_BY` from users to facts

7. **Indexes & Constraints**
   - Unique constraints on Discord IDs, Guild IDs, Channel IDs, Role IDs
   - Composite indexes for common query patterns
   - Full-text search indexes (if supported by Neo4j version)

## Running the Migration

### Prerequisites
- Neo4j database running and accessible
- Configuration file with Neo4j credentials

### Basic Usage

```bash
cd backend/scripts
go run migrate_enhanced_schema.go
```

### Force Re-run

To force re-running the migration (useful for development):

```bash
go run migrate_enhanced_schema.go -force
```

### Build and Run

```bash
# Build
go build -o migrate_enhanced_schema migrate_enhanced_schema.go

# Run
./migrate_enhanced_schema

# Or with force flag
./migrate_enhanced_schema -force
```

## Migration Steps

The migration runs in the following order:

1. **Create Constraints** - Adds unique constraints for data integrity
2. **Create Indexes** - Adds performance indexes
3. **Create Full-Text Indexes** - Adds full-text search (if supported)
4. **Add Relationship Properties** - Enhances existing relationships with temporal/weighted properties
5. **Create User Similarity Relationships** - Calculates and creates similarity relationships
6. **Add Conversation Metadata** - Adds message/participant counts to conversations
7. **Create Fact Relationship Indexes** - Adds indexes for fact queries

## Safety

- The migration is **idempotent** - safe to run multiple times
- Uses `IF NOT EXISTS` clauses where possible
- Checks if migration was already applied (unless `-force` is used)
- Logs all steps for visibility

## Rollback

Currently, there is no automatic rollback. If you need to revert:

1. Manually remove the constraints/indexes
2. Remove relationship properties
3. Delete the migration record: `MATCH (m:Migration {version: 'enhanced_schema_v1'}) DELETE m`

## Verification

After migration, verify with:

```cypher
// Check migration was applied
MATCH (m:Migration {version: 'enhanced_schema_v1'})
RETURN m.applied_at, m.description

// Check constraints
SHOW CONSTRAINTS

// Check indexes
SHOW INDEXES

// Check a sample enhanced relationship
MATCH (u:User)-[r:INTERESTED_IN]->(t:Topic)
WHERE r.strength IS NOT NULL
RETURN u.id, t.name, r.strength, r.interaction_count
LIMIT 5
```

## Notes

- Full-text indexes require Neo4j 4.0+ and may not be available in all versions
- Some migration steps may show warnings for existing constraints/indexes - this is normal
- The migration enhances existing data but doesn't delete anything
- User similarity calculations are done during migration and can be recalculated using repository methods

