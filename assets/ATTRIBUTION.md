# Embedded vocabulary data

`aat-terms-2026-01.csv.gz` contains English terms extracted from the Getty Art &
Architecture Thesaurus (AAT), used under the Open Data Commons Attribution
License (ODC-By) 1.0.

- **Source:** J. Paul Getty Trust, Getty Research Institute
- **Vocabulary:** Art & Architecture Thesaurus (AAT)
- **Archive:** `aat_rel_0126.zip`, published 6 January 2026, from
  <http://aatdownloads.getty.edu/>
  SHA-256 `b8b6425a136c4120ec3901eaf27f197e1a89cd188a42d0fd51f8ad40e3e06af6`
- **This file:**
  SHA-256 `ae6225be3581a4001d96a1907eebf44689d77ce58d48ec77634b21094693f851`
- **License:** Open Data Commons Attribution License (ODC-By) 1.0 —
  <https://opendatacommons.org/licenses/by/1-0/>

## What was extracted

The relational archive's `TERM` and `LANGUAGE_RELS` tables were joined to keep
terms whose language is English: `70051` English, `70052` American English, and
`70053` British English. All three are English terms — leaving the dialects out
cost about 8,000 terms, `place settings` among them, which the check then called
invalid. Preferred terms and variants are both
kept, since a variant is a legitimate AAT term — but which is which is recorded,
because this catalogue accepts the preferred term only.

Each line is the term, followed by the preferred term of the same concept when
the term is a variant:

```
photographs
photos,photographs
```

A term that is preferred for any concept is written alone, even where another
concept holds it as a variant: being preferred somewhere makes it a term to use
as written. Where a concept is preferred in more than one English, the plain
English label is the one named as the replacement, since that is what the live
endpoint returns. Terms were deduplicated without regard to case and quoted
where they contain a comma.

176,629 terms, of which 120,246 are variants pointing at a preferred term.

## A limitation of this format

Getty writes qualified terms as `counters (furniture)`. The relational archive
stores the term as bare `counters` and keeps the qualifier in data this export
does not carry, so the extracted list holds no qualified spellings at all.

Comparing exactly against this list would reject `counters (furniture)`, a tag
taken verbatim from a real export and confirmed valid by the live endpoint. The
application therefore falls back to the base term when a qualified one is
absent, and reports that the bracketed part went unchecked — it confirms the
term, not the qualifier.

## Why the app does not fetch this itself

Getty's download host answers on plain HTTP and has nothing listening on 443,
so that transfer cannot be encrypted, and the app makes no unencrypted
connections. The archive is obtained separately and converted with the app's
"Build list from a Getty archive" button, which reads a local file.

The checksums above are what makes an unencrypted fetch acceptable for building
this bundled copy: the archive was verified against them before extraction, so
the data committed here does not rest on the transport.

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
