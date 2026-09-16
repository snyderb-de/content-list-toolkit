# Embedded vocabulary data

`aat-terms-2026-01.csv.gz` contains English terms extracted from the Getty Art &
Architecture Thesaurus (AAT), used under the Open Data Commons Attribution
License (ODC-By) 1.0.

- **Source:** J. Paul Getty Trust, Getty Research Institute
- **Vocabulary:** Art & Architecture Thesaurus (AAT)
- **Archive:** `aat_rel_0126.zip`, published 6 January 2026, from
  <http://aatdownloads.getty.edu/>
- **License:** Open Data Commons Attribution License (ODC-By) 1.0 —
  <https://opendatacommons.org/licenses/by/1-0/>

## What was extracted

The relational archive's `TERM` and `LANGUAGE_RELS` tables were joined to keep
terms whose language is English (`70051`). Preferred terms and variants are both
kept, since a variant is a legitimate AAT term. Terms were deduplicated without
regard to case and written one per line, quoted where they contain a comma.

169,307 terms.

## A limitation of this format

Getty writes qualified terms as `counters (furniture)`. The relational archive
stores the term as bare `counters` and keeps the qualifier in data this export
does not carry, so the extracted list holds no qualified spellings at all.

Comparing exactly against this list would reject `counters (furniture)`, a tag
taken verbatim from a real export and confirmed valid by the live endpoint. The
application therefore falls back to the base term when a qualified one is
absent, and reports that the bracketed part went unchecked — it confirms the
term, not the qualifier.

## Why it is dated, and what that means

Getty froze these relational archives in January 2026 and has said they will not
be refreshed; the live SPARQL endpoint at `vocab.getty.edu` is the current
source. Getty continues to revise the thesaurus, so this copy can accept a term
Getty has since renamed.

A known example: this archive lists `landscapes` as the preferred label of AAT
subjects 300015636 and 300008626. The live endpoint holds no AAT label of that
spelling today. Anything answered from this copy carries that caveat in the
application and in its reports.

## Regenerating

Use **Download list from Getty** on the Getty Tags screen, which performs the
same extraction against whatever archive Getty currently publishes.
