-- Enums
DO $$ BEGIN
    CREATE TYPE user_role AS ENUM ('parent', 'kid');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE task_type AS ENUM ('adhoc', 'daily_recurring', 'accumulation');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE task_status AS ENUM ('pending', 'in_progress', 'submitted', 'approved', 'rejected');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Family / Households
CREATE TABLE IF NOT EXISTS families (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Profiles (Parents & Kids)
CREATE TABLE IF NOT EXISTS profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    full_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE,
    role user_role NOT NULL,
    pin_hash VARCHAR(255) NOT NULL,
    current_balance NUMERIC(10, 2) DEFAULT 0.00 CHECK (current_balance >= 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Task Configurations
CREATE TABLE IF NOT EXISTS task_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    created_by UUID REFERENCES profiles(id),
    title VARCHAR(255) NOT NULL,
    description TEXT,
    task_type task_type NOT NULL DEFAULT 'adhoc',
    reward_amount NUMERIC(8, 2) NOT NULL CHECK (reward_amount >= 0),
    target_units INT DEFAULT 1,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Active Task Instances & Kid Progress
CREATE TABLE IF NOT EXISTS task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_definition_id UUID NOT NULL REFERENCES task_definitions(id) ON DELETE CASCADE,
    assigned_to UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    status task_status NOT NULL DEFAULT 'pending',
    current_progress_units INT DEFAULT 0,
    notes TEXT,
    submitted_at TIMESTAMP WITH TIME ZONE,
    reviewed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Financial Ledger
CREATE TABLE IF NOT EXISTS ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    kid_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    task_log_id UUID REFERENCES task_logs(id),
    amount NUMERIC(8, 2) NOT NULL,
    transaction_type VARCHAR(20) CHECK (transaction_type IN ('EARNED', 'PAYOUT', 'ADJUSTMENT')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for Speed
CREATE INDEX IF NOT EXISTS idx_profiles_family ON profiles(family_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_assigned_status ON task_logs(assigned_to, status);
CREATE INDEX IF NOT EXISTS idx_ledger_kid ON ledger(kid_id);
