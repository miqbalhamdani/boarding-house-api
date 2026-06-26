# Module 02: Room Management

## Goal
Allow owner to create, view, update, and delete rooms in their own workspace.

## Scope
Included:
- room list
- room create
- room detail
- room update
- soft delete room
- filter by status

Excluded:
- house/property management
- room photos
- maintenance requests

## Main Table
- rooms

## API Endpoints
- `GET /owner/rooms`
- `POST /owner/rooms`
- `GET /owner/rooms/{room_id}`
- `PATCH /owner/rooms/{room_id}`
- `DELETE /owner/rooms/{room_id}`

## Business Rules
- Room number must be unique per owner.
- Only rooms under authenticated owner can be accessed.
- Room status options: available, reserved, occupied, maintenance, inactive.
- Occupied or reserved rooms should not be deleted unless business decision allows soft delete only.

## Acceptance Criteria
- Owner can manage their own rooms.
- Owner cannot access another owner's rooms.
- Room list supports search and status filter.
