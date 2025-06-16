CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    job_id TEXT NOT NULL,
    status TEXT NOT NULL,
    parameters JSONB NOT NULL,
    output_format TEXT,
    output_path TEXT,
    output_size BIGINT,
    patient_count INTEGER,
    error_message TEXT,
    s3_prefix TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
); 