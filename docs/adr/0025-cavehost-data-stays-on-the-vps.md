---
status: accepted, departs from ADR 0008 for CaveHost only
---

# CaveHost fetches the Spreadsheet, and its data stays on the VPS

CaveHost puts a Learner's own HTML page online with its data in a Google Spreadsheet. Cave fetches the Spreadsheet on the server, every hour or on Sync now, and serves each ticked sheet's rows from the Deployment's own host, as the Lab serves `data.json`; the page never fetches from Google. A page's CSP allows connections to Cave only (ADR 0024). Opening it to `docs.google.com`, `script.google.com` and `*.googleusercontent.com` would let any Learner page post visitors' input to a Google Form or an Apps Script, a ready phishing collector, and a page that read the Spreadsheet itself would break, unchecked, whenever the Spreadsheet changed shape.

Cave fetches the whole Spreadsheet as .xlsx, not CSV: the xlsx export carries every sheet with its name and each formula's computed value, which CSV, one sheet per link, and Cave, without a Google sign-in, cannot list. Clean-up then reads it as it reads a Workbook, with empty columns dropped, merged cells filled down and error cells left empty, so no dashboard engine comes back (ADR 0018). The Learner confirms each sheet's first Clean-up; later Syncs apply its recipe without asking.

CaveHost data, the last good rows of each sheet and the page with its 3 earlier copies, is kept on the VPS disk, not in R2. ADR 0008 put Lab data in R2 because tens of thousands of Learners with 1 GB each outgrow one disk. CaveHost is a premium Plan for a few selected Learners, each Sync overwrites the last, and Cave keeps only one copy of each sheet, so the disk (111 GB free in October 2026) holds it with no Storage allowance. Each Sync is still refused above 50 MB, so one huge Spreadsheet cannot run the VPS out of memory and take the LMS down with it. If CaveHost is ever sold widely, its data moves to R2 as the Lab's did.

Rejected: (1) the page fetching a published CSV straight from Google, as first suggested, for the CSP and checking reasons above; (2) Google sign-in for private Spreadsheets, which needs Google's app verification and token storage, while the data is public on the Deployment anyway; (3) CSV export, one sheet per link and no sheet names; (4) adding each Sync as a Part (ADR 0022), since a Spreadsheet is one living table, not a stream of reports.
