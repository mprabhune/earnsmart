# System Architecture & Blueprint: EarnSmart

**Version:** 1.1.0  
**Target Execution Engine:** Antigravity CLI  
**Application Domain:** Family Task & Financial Rewards Platform  

---

## 1. Executive Summary & Configuration

**EarnSmart** is a cross-platform, lightweight rewards application engineered in **Go (Golang)**. It allows parents to manage household tasks and financial incentives, and gives kids a desktop-friendly interface to log progress and track real-time cash balances.

### Key Requirements Implemented
1. **Core Stack:** Go REST API + Supabase PostgreSQL + Svelte/React Web Client + Android Native/PWA App.
2. **Simplified Kid Authentication:** 4-digit PIN authentication per kid profile (no complex passwords or email verification required for kids).
3. **Flexible Reward Rules:** Supports one-time (ad-hoc), daily recurring, and threshold-based accumulation tasks (e.g., "Complete 5 math pages/day for 10 days = $1.00", "30 mins piano practice = $1.00").
4. **Dynamic Parent Controls:** Parents can dynamically create, edit, disable, or reconfigure dollar values and task rules on the fly.
5. **Zero Local Persistence:** Complete state management in cloud PostgreSQL with secure, memory-only session tokens.

---

## 2. Infrastructure & Hosting Architecture (100% Free Tier)

| Layer | Service / Tech | Specifications |
| :--- | :--- | :--- |
| **Backend API** | **Go (Golang)** on Render Free Tier | Light memory footprint (<25 MB RAM overhead), containerized execution, native TLS endpoint. |
| **Database** | **Supabase (PostgreSQL)** | Free 500 MB cloud PostgreSQL DB with Row Level Security (RLS) enabled. |
| **Web Frontend** | **Vercel** or **Netlify** | Static/SPA hosting with free SSL deployment for desktop browser access. |
| **Parent Android** | **Flutter** or **Bubblewrap (TWA)** | Mobile wrapper rendering parent portal natively with zero server cost. |

---

## 3. Data Model & Database Schema

```sql
-- Enums
CREATE TYPE user_role AS ENUM ('parent', 'kid');
CREATE TYPE task_type AS ENUM ('adhoc', 'daily_recurring', 'accumulation');
CREATE TYPE task_status AS ENUM ('pending', 'in_progress', 'submitted', 'approved', 'rejected');

-- Family / Households
CREATE TABLE families (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Profiles (Parents & Kids)
CREATE TABLE profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID REFERENCES families(id) ON DELETE CASCADE,
    full_name VARCHAR(100) NOT NULL,
    role user_role NOT NULL,
    -- 4-digit PIN stored as a bcrypt hash for kids, standard password for parents
    pin_hash VARCHAR(255) NOT NULL,
    current_balance NUMERIC(10, 2) DEFAULT 0.00 CHECK (current_balance >= 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Task Configurations (Parent-Configurable Rules)
CREATE TABLE task_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID REFERENCES families(id) ON DELETE CASCADE,
    created_by UUID REFERENCES profiles(id),
    title VARCHAR(255) NOT NULL,
    description TEXT,
    task_type task_type NOT NULL DEFAULT 'adhoc',
    reward_amount NUMERIC(8, 2) NOT NULL CHECK (reward_amount >= 0),
    
    -- Accumulation Rules (e.g., target_streak = 10 days for $1 reward)
    target_units INT DEFAULT 1, -- e.g., 10 days or 5 pages
    is_active BOOLEAN DEFAULT TRUE,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Active Task Instances & Kid Progress
CREATE TABLE task_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_definition_id UUID REFERENCES task_definitions(id) ON DELETE CASCADE,
    assigned_to UUID REFERENCES profiles(id) ON DELETE CASCADE,
    status task_status NOT NULL DEFAULT 'pending',
    
    -- Accumulation Tracking
    current_progress_units INT DEFAULT 0,
    
    notes TEXT,
    submitted_at TIMESTAMP WITH TIME ZONE,
    reviewed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Financial Ledger (Immutable Transaction Records)
CREATE TABLE ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID REFERENCES families(id) ON DELETE CASCADE,
    kid_id UUID REFERENCES profiles(id) ON DELETE CASCADE,
    task_log_id UUID REFERENCES task_logs(id),
    amount NUMERIC(8, 2) NOT NULL,
    transaction_type VARCHAR(20) CHECK (transaction_type IN ('EARNED', 'PAYOUT', 'ADJUSTMENT')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for Speed
CREATE INDEX idx_profiles_family ON profiles(family_id);
CREATE INDEX idx_task_logs_assigned_status ON task_logs(assigned_to, status);
CREATE INDEX idx_ledger_kid ON ledger(kid_id);
```

---

## 4. Go Backend Architecture & API Routes

### 4.1 Project Directory Structure
```text
earnsmart-backend/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── config/
│   ├── database/
│   ├── handlers/
│   │   ├── auth.go
│   │   ├── kid.go
│   │   └── parent.go
│   ├── middleware/
│   │   └── auth_jwt.go
│   └── models/
├── Dockerfile
├── go.mod
└── go.sum
```

### 4.2 Endpoint Specifications

#### Auth & Sessions
* `POST /api/v1/auth/parent/login` – Login parent via email & password $ightarrow$ Returns JWT.
* `POST /api/v1/auth/kid/login` – Login kid via Profile ID & **4-digit PIN** $ightarrow$ Returns lightweight JWT.

#### Parent Endpoints (Android App / Web Control)
* `GET /api/v1/parent/tasks` – List all task definitions and active configurations.
* `POST /api/v1/parent/tasks` – Create new task rule (Title, Amount, Type, Accumulation target).
* `PUT /api/v1/parent/tasks/:id` – **Reconfigure task** (change payout amount, target days, or title).
* `GET /api/v1/parent/approvals` – Fetch all task completions waiting for approval.
* `POST /api/v1/parent/approvals/:id/review` – Approve or reject submission (`status: approved` triggers atomic SQL transaction updating total balance and appending to `ledger`).
* `GET /api/v1/parent/summary` – Summary of each kid's earned dollars, completed tasks, and current streaks.

#### Kid Endpoints (Desktop Web Interface)
* `GET /api/v1/kid/dashboard` – Fetch assigned daily, ad-hoc, and accumulation tasks along with current total balance.
* `POST /api/v1/kid/tasks/:id/log` – Record progress (e.g., mark daily math done or increment unit count).
* `POST /api/v1/kid/tasks/:id/submit` – Submit completed task/streak for parent review.

---

## 5. Logic Flow Examples

### Accumulation Rules Example (Math 5 Pages for 10 Days = $1.00)
1. Parent creates a `task_definition`:
   * `task_type`: `accumulation`
   * `target_units`: `10` (days)
   * `reward_amount`: `$1.00`
2. Each day, Kid logs task progress (+1 unit).
3. Once `current_progress_units == 10`, status transitions to `submitted`.
4. Parent reviews and approves on Android app:
   * Status $ightarrow$ `approved`
   * Balance incremented by `$1.00`
   * Ledger entry recorded.

---

## 6. Antigravity CLI Execution Commands

To execute this project with Antigravity CLI, run the following sequential setup prompts:

1. **Database Deployment:**
   `antigravity db init --provider supabase --schema ./schema.sql`
2. **Go REST API Initialization:**
   `antigravity backend create --lang golang --framework chi --dockerfile`
3. **Frontend Generation:**
   `antigravity frontend create --template svelte-spa --output ./web-client`
4. **Build Android APK Bundle:**
   `antigravity mobile build --type twa --url https://your-vercel-domain.app`
