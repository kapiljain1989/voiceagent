// Database client — will be configured when PostgreSQL is added to docker-compose.
// Prisma 7 requires adapter configuration for PrismaClient constructor.
// See: https://www.prisma.io/docs/orm/prisma-client/setup-and-configuration

export const DATABASE_URL = process.env.DATABASE_URL || "postgresql://voiceagent:voiceagent@postgres:5432/voiceagent";
