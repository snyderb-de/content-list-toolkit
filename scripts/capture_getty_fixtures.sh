#!/usr/bin/env bash
#
# Captures real responses from Getty's SPARQL endpoint for use as test
# fixtures.
#
# The tests for the live vocabulary source run against recorded responses so
# they need no network. A fixture written by hand encodes what someone assumed
# the endpoint returns; a captured one records what it actually returns, and
# the difference is where the bugs live. This script exists so the fixtures can
# be refreshed by anyone on a machine that can reach vocab.getty.edu.
#
# The queries below are the ones the application sends. If buildAATLookupQuery
# or buildAATSuggestQuery changes in getty_vocab_sparql.go, change them here to
# match, or the fixtures stop describing the real thing.
#
# Usage:  ./scripts/capture_getty_fixtures.sh [output-dir]
#         Defaults to testing/getty-fixtures/

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-$ROOT_DIR/testing/getty-fixtures}"
ENDPOINT="https://vocab.getty.edu/sparql.json"
AAT="http://vocab.getty.edu/aat/"

mkdir -p "$OUT_DIR"

lookup_query() {
  local term="$1"
  local lowered
  lowered="$(printf '%s' "$term" | tr '[:upper:]' '[:lower:]')"
  cat <<QUERY
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX xl: <http://www.w3.org/2008/05/skos-xl#>
PREFIX luc: <http://www.ontotext.com/owlim/lucene#>
SELECT ?s ?matched ?label WHERE {
  ?s luc:term "$term" ;
     skos:inScheme <$AAT> ;
     xl:prefLabel/xl:literalForm ?label .
  { ?s xl:prefLabel/xl:literalForm ?matched }
  UNION
  { ?s xl:altLabel/xl:literalForm ?matched }
  FILTER(lcase(str(?matched)) = "$lowered")
} LIMIT 60
QUERY
}

suggest_query() {
  cat <<QUERY
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX xl: <http://www.w3.org/2008/05/skos-xl#>
PREFIX luc: <http://www.ontotext.com/owlim/lucene#>
SELECT ?label WHERE {
  ?s luc:term "$1" ;
     skos:inScheme <$AAT> ;
     xl:prefLabel/xl:literalForm ?label .
} LIMIT 60
QUERY
}

capture() {
  local name="$1" query="$2" path="$OUT_DIR/$1.json"
  printf '  %-34s ' "$name.json"
  local code
  code="$(curl -s -G "$ENDPOINT" \
    --data-urlencode "query=$query" \
    -H 'Accept: application/sparql-results+json' \
    -o "$path" -w '%{http_code}' --max-time 45)"
  if [[ "$code" != "200" ]]; then
    echo "HTTP $code — not saved"
    rm -f "$path"
    return 1
  fi
  echo "HTTP 200, $(wc -c < "$path" | tr -d ' ') bytes"
}

echo "Capturing Getty responses to $OUT_DIR"
echo

# A term that exists, to pin the shape of a match and the several languages a
# single concept comes back in.
capture "lookup-found" "$(lookup_query 'aerial photographs')"

# A term Getty does not hold. Both this and the one above were confirmed
# against the live endpoint; "landscapes" is not an AAT label, though Getty's
# own January 2026 archive still lists it.
capture "lookup-absent" "$(lookup_query 'landscapes')"

# A hyphenated term, because Lucene treats some punctuation as syntax and this
# is where that would show.
capture "lookup-hyphenated" "$(lookup_query 'black-and-white photographs')"

# Candidates for a term that failed, which is what the suggestions are built
# from.
capture "suggest-counters" "$(suggest_query 'counters')"

echo
echo "Done. Review the files, then update the fixtures in"
echo "getty_vocab_sparql_test.go to match what was captured."
