# User Flows

## 1. Owner Registration and Login

```text
Owner opens register page
→ enters business and account details
→ system creates owner workspace
→ owner logs in
→ owner sees dashboard
```

## 2. Room Creation

```text
Owner opens Rooms page
→ clicks Add Room
→ enters room number, name, rent, status
→ system creates room under owner's workspace
→ room appears in room list
```

## 3. Tenant Creation

```text
Owner opens Tenants page
→ clicks Add Tenant
→ enters tenant profile
→ system creates tenant with pending_payment status
→ tenant appears in tenant list
```

## 4. Tenant Onboarding

```text
Owner opens Onboarding page
→ selects tenant
→ selects available room
→ enters start date, monthly rent, due day
→ system creates room assignment
→ system creates first rent bill
→ system changes room status to reserved
→ tenant status remains pending_payment
```

Example:

```text
Tenant: Budi
Room: 101
Monthly rent: Rp 2,000,000
Start date: 10 July 2026
Due day: 10

System creates:
- room assignment: pending_payment
- bill: July 2026, Rp 2,000,000, unpaid
- room: reserved
```

## 5. Tenant Pays First Bill Through Gateway

```text
Tenant logs in to tenant portal
→ opens Bills page
→ sees first unpaid bill
→ clicks Pay Now
→ system creates gateway checkout transaction
→ tenant is redirected/opened to gateway checkout page
→ tenant pays full amount
→ gateway sends webhook to system
→ system verifies webhook
→ system marks gateway transaction as paid
→ system creates payment record
→ system marks bill as paid
→ system activates tenant and room assignment
→ system changes room status to occupied
```

## 6. Owner Records Manual Payment Backup

```text
Owner opens Bill Detail
→ clicks Record Manual Payment
→ enters amount equal to bill amount
→ enters payment method and reference
→ system validates full amount
→ system creates payment record
→ system marks bill as paid
→ if this is first bill, tenant and room assignment become active
```

## 7. Automatic Monthly Billing

```text
Scheduled job runs on first day of month
→ system finds all active room assignments
→ system checks whether bill already exists for assignment and month
→ if not exists, system creates monthly rent bill
→ bill status is unpaid
```

Duplicate prevention:

```text
room_assignment_id + billing_month must be unique
```

## 8. Monthly Bill Payment Through Gateway

```text
Tenant opens current unpaid bill
→ clicks Pay Now
→ system creates checkout link
→ bill status becomes gateway_pending
→ tenant pays through gateway
→ webhook confirms successful payment
→ bill becomes paid
→ payment record is created
```

## 9. Gateway Payment Failed or Expired

```text
Tenant opens checkout but does not pay
→ gateway checkout expires
→ gateway sends expired/failed webhook or system checks transaction status
→ gateway transaction becomes expired/failed
→ bill returns to unpaid or overdue
→ tenant can create a new payment link
```

## 10. Overdue Bill Flow

```text
System checks unpaid bills daily
→ if today > due_date and status is unpaid
→ bill status becomes overdue
→ owner sees bill in overdue list
→ tenant sees overdue warning in portal
```

## 11. Tenant Move Out

```text
Owner opens tenant detail
→ clicks Move Out
→ enters move-out date
→ system ends room assignment
→ tenant status becomes moved_out
→ room status becomes available or maintenance
```
