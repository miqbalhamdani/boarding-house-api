# Product Requirements Document

## Product Name
Room Rental Management SaaS

## Product Summary
A multi-tenant room rental management application for property owners to manage rooms, tenants, tenant onboarding, automated monthly rent billing, payment gateway collection, manual payment recording, dashboards, and a tenant portal.

The product supports multiple property owners on the same platform. Each owner has an isolated workspace. Owners can only access their own rooms, tenants, bills, gateway transactions, payments, and reports.

## MVP Scope
The MVP focuses on room-based rental management, not full property or house management.

Included:
- Owner/admin authentication
- Tenant authentication for portal access
- Multi-tenant owner account isolation
- Room management
- Tenant management
- Tenant onboarding and room assignment
- Automatic monthly rent bill generation
- Full-payment-only billing
- Payment gateway checkout for tenants
- Payment gateway webhook handling
- Manual payment recording as backup
- Dashboard overview
- Tenant portal for viewing bills, opening payment links, and viewing payment history

Excluded from MVP:
- Deposit bills
- Partial payments
- Multi-property/house management
- Complex role and permission system
- Maintenance requests
- Contract signing
- Advanced accounting
- Automated WhatsApp/SMS integration
- Refund automation
- Owner payout/split settlement automation

## User Types

### Owner/Admin
The property owner or admin. In MVP, each owner account is treated as one isolated workspace.

Capabilities:
- Manage rooms
- Manage tenants
- Assign tenants to rooms
- Generate and view monthly bills
- Create payment links for unpaid bills
- View payment gateway transaction status
- Record manual full payments when needed
- View dashboard and reports

### Tenant
The person renting a room.

Capabilities:
- Login to tenant portal
- View assigned room
- View monthly bills
- Open payment gateway checkout link
- Pay full bill amount through payment gateway
- View payment status and payment history

## Core Concepts

### Tenant Isolation
Every main business record belongs to an owner account using `owner_id`.

Examples:
- Room belongs to owner
- Tenant belongs to owner
- Bill belongs to owner
- Payment gateway transaction belongs to owner
- Payment belongs to owner

One owner must never see, update, or delete another owner's data.

### Room-Based Rental
The MVP does not require a house/property table. Rooms are the main rental units.

### Automatic Monthly Billing
The system automatically creates one monthly rent bill for every active room assignment.

Recommended MVP behavior:
- First rent bill is created during tenant onboarding.
- Future monthly bills are generated automatically on the first day of every month.
- The system must not create duplicate bills for the same `room_assignment_id` and `billing_month`.

### Full Payment Only
A bill can only be paid in full. Partial payments are not allowed.

Rules:
- Payment gateway checkout amount must equal bill amount.
- Manual payment amount must equal bill amount.
- One bill can have only one successful payment.

### No Deposit Bill
The MVP only bills monthly room rent.

### Payment Gateway Integration
Tenants can pay unpaid bills through a payment gateway checkout link.

The system must:
- Create a gateway transaction for an unpaid bill.
- Store gateway reference IDs and checkout/payment URLs.
- Receive payment notifications through webhooks.
- Verify webhook authenticity.
- Mark the bill as paid only after receiving a successful paid/settlement status.
- Keep gateway webhook events for audit/debugging.

The documentation uses a provider-neutral design so the implementation can use Midtrans, Xendit, Stripe, or another gateway.

## Main User Journey
1. Owner registers and logs in.
2. Owner creates rooms.
3. Owner creates tenant.
4. Owner assigns tenant to an available room.
5. System creates the first monthly rent bill.
6. Tenant opens the tenant portal.
7. Tenant views unpaid bill.
8. Tenant clicks Pay Now.
9. System creates a payment gateway checkout session/link.
10. Tenant completes full payment on gateway page.
11. Gateway sends webhook to the system.
12. System verifies webhook and marks bill as paid.
13. System creates a payment record.
14. Tenant and room assignment become active after first bill is paid.
15. Future monthly rent bills are generated automatically.
16. Owner monitors unpaid, overdue, paid, and gateway-pending bills from dashboard.

## Success Criteria
- Owner can create and manage rooms.
- Owner can create and manage tenants.
- Owner can assign one tenant to one room.
- System creates first and future rent bills correctly.
- System automatically generates monthly bills without duplicates.
- Tenant can pay a bill through payment gateway checkout.
- Gateway webhook can mark a bill as paid.
- Manual full payment recording is available as backup.
- System prevents partial payments.
- Dashboard shows accurate rooms, tenants, bills, payments, and gateway status.
- Tenant can view bills and payment history.
- Data is isolated per owner.

## MVP Non-Functional Requirements
- Responsive web application.
- Secure authentication.
- Owner data isolation enforced at API and database query level.
- Secure payment webhook verification.
- Idempotent webhook handling to prevent double payment records.
- All money values stored as integers.
- Clear validation messages.
- Audit-friendly timestamps on all main tables.
