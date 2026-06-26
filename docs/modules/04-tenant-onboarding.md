# Module 04: Tenant Onboarding

## Goal
Assign a tenant to a room and create the first monthly rent bill.

## Scope
Included:
- select tenant
- select available room
- set start date
- set monthly rent
- set payment due day
- create room assignment
- create first rent bill
- reserve room until first bill is paid

Excluded:
- deposit bill
- contract signing
- document upload

## Main Tables
- tenants
- rooms
- room_assignments
- bills

## API Endpoints
- `POST /owner/onboarding/assign-room`
- `POST /owner/onboarding/{room_assignment_id}/cancel`

## Business Rules
- Only available rooms can be assigned.
- Tenant must not already have active or pending assignment.
- Room must not already have active or pending assignment.
- First bill is created immediately.
- Tenant status becomes pending_payment.
- Room status becomes reserved.
- Room assignment status becomes pending_payment.

## Acceptance Criteria
- Owner can assign tenant to available room.
- System creates first unpaid rent bill.
- Room becomes reserved.
- Duplicate assignment is prevented.
