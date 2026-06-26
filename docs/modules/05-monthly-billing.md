# Module 05: Monthly Billing

## Goal
Generate and manage monthly rent bills.

## Scope
Included:
- first bill from onboarding
- automatic monthly bill generation
- manual generate bills backup action
- bill list
- bill detail
- overdue status update

Excluded:
- deposit bills
- partial payments
- utility bills
- late fees

## Main Table
- bills

## API Endpoints
- `GET /owner/bills`
- `GET /owner/bills/{bill_id}`
- `POST /owner/bills/generate-monthly`
- `POST /owner/bills/mark-overdue`

## Automatic Generation Rule
The system automatically creates one monthly rent bill for every active room assignment.

Recommended MVP schedule:

```text
Run daily.
If today is day 1 of the month:
  generate bills for current month
```

## Business Rules
- Only active room assignments receive automatic monthly bills.
- One assignment can only have one bill per billing month.
- Bill amount uses `room_assignments.monthly_rent`.
- Due date uses `payment_due_day`.
- Status starts as unpaid.
- If current date passes due date and bill is unpaid, status becomes overdue.

## Acceptance Criteria
- Bills generate automatically without duplicates.
- Manual generation does not create duplicates.
- Bill amount and due date are correct.
- Overdue bills are detected correctly.
