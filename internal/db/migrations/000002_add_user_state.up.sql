-- Add user state field to users table
ALTER TABLE users ADD COLUMN state VARCHAR(20) NOT NULL DEFAULT 'active';

-- Add force_password_reset column to users table
ALTER TABLE users ADD COLUMN force_password_reset BOOLEAN NOT NULL DEFAULT false; 