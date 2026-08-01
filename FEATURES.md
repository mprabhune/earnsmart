# EarnSmart — Feature Specification

> This document tracks all implemented features and their expected behaviour.
> Use this as the reference when building a new version (native app, local-first, etc.)
> so that look, feel, and logic stay consistent.

---

## 1. Authentication

### 1.1 Parent Registration
- Fields: Family Name, Full Name, Email, Password, 4-digit PIN
- Creates a new `family` record and a `parent` profile in one transaction
- Both `password_hash` and `pin_hash` are stored (bcrypt)
- Email must be unique across all profiles

### 1.2 Parent Login
- Two methods available via tabs: **Password** and **PIN**
- Either method authenticates successfully against the same account
- Returns a JWT (24 hr expiry) + profile + family object

### 1.3 Kid Login
- Fields: Full Name + 4-digit PIN
- Looks up kid by full name within the family (case-sensitive match)
- Returns JWT + profile + family object

### 1.4 Forgot / Reset Credentials
- Parent enters their email → server generates a 6-digit reset code
- Code is displayed on-screen (no email service — by design for simplicity)
- Parent enters email + code + new password and/or new PIN
- Code expires after 15 minutes
- Clears token after successful reset

### 1.5 Session Management
- JWT stored in `localStorage`
- On page/app load: token is verified against the API before restoring session
- On 401 response: only logs out if a token was previously set (prevents logout loop on failed login)

---

## 2. Parent Dashboard

### 2.1 Navigation Tabs
- **Kids** — default tab on login
- **Approvals** — tasks submitted by kids awaiting review
- **Summary** — earnings overview per kid
- **Payout** — record a cash payout to a kid

### 2.2 Notification Banner
- Loads automatically on parent login
- Shows tasks that have been **pending for 24+ hours**
- Shows tasks that are **overdue** (past due date, not yet approved)
- Format: `⚠️ N overdue: [kid] — [task]. ⏰ N pending 24h+: [kid] — [task].`
- Banner hidden if no alerts

---

## 3. Kids Management

### 3.1 Kids List
- Shows all kids in the family with their current balance
- Each kid card is tappable → opens Kid Detail view
- "Tap to view tasks →" hint on each card

### 3.2 Add Kid
- Fields: Full Name, 4-digit PIN
- PIN is bcrypt-hashed before storage
- After creation: shows the kid's UUID (for reference)

### 3.3 Kid Detail View
- Shows kid's name and current balance at top
- Date range filter (From / To) with a Clear button
- Lists all task logs assigned to this kid, filtered by date range
- Each task log shows:
  - Task title, status badge, overdue badge (if applicable)
  - Reward per unit × target units = total payout
  - Due date (red if overdue)
  - Completion timestamp (if logged)
  - Progress bar for accumulation/multi-unit tasks
- "Assign Task" button → opens Add Task modal pre-selected to this kid
- Back button → returns to Kids List

---

## 4. Tasks

### 4.1 Task Types
| Type | Behaviour |
|------|-----------|
| `adhoc` | One-time task, single completion |
| `daily_recurring` | Repeats daily, each day is a new log |
| `accumulation` | Progress tracked in units (e.g. pages read) |

### 4.2 Task Definition Fields
- Title (required)
- Description (optional)
- Task Type
- Reward per Unit ($) — e.g. $0.20/page
- Target Units — e.g. 5 pages → total reward = $0.20 × 5 = $1.00
- Due Date (optional deadline, shown in red when overdue)
- Assign to specific kid (optional — blank means available to all kids)

### 4.3 Task Management (Parent)
- Create, Edit, Delete task definitions
- Edit: can update title, reward, units, due date, active/inactive status
- Inactive tasks are not shown to kids

### 4.4 Task Logs
- A task log is created when a kid starts or is assigned a task
- Statuses: `pending` → `in_progress` → `submitted` → `approved` / `rejected`

---

## 5. Kid Dashboard

### 5.1 Dashboard View
- Shows kid's name and current balance prominently
- Lists all task logs assigned to this kid
- Each task shows: title, status badge, reward info, progress bar
- Action buttons per status:
  - `pending` / `in_progress` → **Log Progress** button
  - `in_progress` (sufficient units) → **Submit** button
  - `submitted` → "Awaiting approval" (no action)
  - `approved` / `rejected` → status only

### 5.2 Log Progress
- Fields:
  - Units to add (number)
  - Completion timestamp (required — datetime picker, pre-filled to now)
  - Notes (optional text)
  - Screenshot / Proof image (optional, image file, max 3MB, stored as base64)
- Updates `current_progress_units` on the task log

### 5.3 Submit Task
- Marks task log as `submitted`
- Triggers approval queue on parent side

---

## 6. Approvals (Parent)

- Lists all task logs with status `submitted`
- Each approval card shows:
  - Kid name, task title, task type
  - Units completed / target units
  - Proof image thumbnail (if uploaded) — tappable to view full size
  - Completion timestamp
  - Kid's notes
- Actions: **Approve** / **Reject** with optional review notes
- On approval: kid's `current_balance` is credited with `reward_amount × target_units`
- Ledger entry (`EARNED`) is created on approval

---

## 7. Summary (Parent)

- Shows per-kid summary card:
  - Current balance
  - Total earned (all time)
  - Completed tasks count
  - Pending tasks count

---

## 8. Payout (Parent)

- Parent selects a kid and enters a payout amount
- Deducts from kid's `current_balance`
- Creates a ledger entry (`PAYOUT`)
- Balance cannot go below $0 (enforced by DB constraint)

---

## 9. Financial Ledger

- Immutable transaction log
- Transaction types: `EARNED`, `PAYOUT`, `ADJUSTMENT`
- Every balance change is recorded with kid ID, family ID, amount, and linked task log (if applicable)

---

## 10. UI / UX Patterns

### Colours
| Token | Value | Usage |
|-------|-------|-------|
| `--green` | `#1a6b3c` | Primary brand, nav, buttons |
| `--green2` | `#228b4e` | Hover states |
| `--gold` | `#f5a623` | Accent, CTA buttons |
| `--red` | `#dc2626` | Errors, overdue indicators |
| `--bg` | `#f4f6f4` | Page background |
| `--card` | `#ffffff` | Card backgrounds |

### Components
- **Cards**: white, 12px radius, subtle shadow, `card-row` for horizontal layout
- **Badges**: `green` (active/approved), `gold` (pending/submitted), `red` (rejected/overdue), `gray` (inactive)
- **Modals**: full-screen overlay, centred modal box, `closeModalOutside` dismisses on backdrop tap
- **Tabs**: horizontal pill-style tabs, active tab highlighted green
- **Progress bar**: green fill bar for accumulation tasks
- **Alert banners**: inline success (green) / error (red) messages within forms
- **Empty states**: centred icon + message when list is empty

### Navigation
- Sticky top nav bar with logo and logout button
- Single-page app — screens shown/hidden via `display` toggle
- No back-button stack (explicit back buttons where needed)

### Responsive
- Mobile-first, works on 360px+ screens
- Cards and modals are full-width on small screens
- `form-grid` uses 2-column layout on wider screens

---

## 11. Backend API Summary

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/parent/register` | None | Register parent + family |
| POST | `/api/v1/auth/parent/login` | None | Login with password or PIN |
| POST | `/api/v1/auth/kid/login` | None | Kid login by name + PIN |
| POST | `/api/v1/auth/forgot` | None | Request reset code |
| POST | `/api/v1/auth/reset` | None | Reset password/PIN with code |
| GET | `/api/v1/parent/kids` | Parent JWT | List all kids |
| POST | `/api/v1/parent/kids` | Parent JWT | Create a kid |
| GET | `/api/v1/parent/kids/{id}/tasks` | Parent JWT | Kid's task logs (date filter) |
| GET | `/api/v1/parent/tasks` | Parent JWT | List task definitions |
| POST | `/api/v1/parent/tasks` | Parent JWT | Create task definition |
| PUT | `/api/v1/parent/tasks/{id}` | Parent JWT | Update task definition |
| DELETE | `/api/v1/parent/tasks/{id}` | Parent JWT | Delete task definition |
| GET | `/api/v1/parent/approvals` | Parent JWT | List submitted task logs |
| POST | `/api/v1/parent/approvals/{id}/review` | Parent JWT | Approve or reject |
| GET | `/api/v1/parent/summary` | Parent JWT | Per-kid earnings summary |
| POST | `/api/v1/parent/payout` | Parent JWT | Record a payout |
| GET | `/api/v1/parent/notifications` | Parent JWT | Tasks pending 24h+ / overdue |
| GET | `/api/v1/kid/dashboard` | Kid JWT | Kid's task list + balance |
| POST | `/api/v1/kid/tasks/{id}/log` | Kid JWT | Log progress on a task |
| POST | `/api/v1/kid/tasks/{id}/submit` | Kid JWT | Submit task for approval |

---

## 12. Deployment (current)

- **Backend**: Go (chi router), deployed on Render (free tier, Docker)
- **Database**: PostgreSQL on Supabase (free tier, session pooler for IPv4)
- **Frontend**: Embedded in Go binary via `//go:embed` — served from same server
- **Android**: TWA (Trusted Web Activity) via Bubblewrap, points to Render URL
- **Schema**: Auto-applied on startup (`schema.sql`), idempotent (`IF NOT EXISTS`)

---

## 13. Planned / Backlog

- [ ] Push notifications (currently banner-only, no push)
- [ ] Multi-family / SaaS mode with proper row-level security
- [ ] Local-first version (SQLite, no cloud dependency)
- [ ] Native Android app (Kotlin or Flutter)
- [ ] Task templates (pre-built common tasks)
- [ ] Recurring task auto-reset (daily tasks auto-create new log each day)
- [ ] Kid earnings history / ledger view
- [ ] Parent activity log (who approved what, when)
