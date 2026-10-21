#!/usr/bin/env bash
QUERIES=(
  "What is RAG?"
  "What are the limitations of naive RAG?"
  "How does retrieval work in RAG systems?"
  "What is the difference between naive and advanced RAG?"
  "What evaluation methods are used for RAG?"
)

TOP_K_VALUES=(3 5 10)

echo "======================================"
echo "Ablation Study — top_k vs Answer Quality"
echo "======================================"
echo ""

for top_k in "${TOP_K_VALUES[@]}"; do
  echo "--- top_k=$top_k ---"
  total_ms=0
  count=0

  for query in "${QUERIES[@]}"; do
    result=$(curl -s -X POST http://localhost:8080/query \
      -H "Content-Type: application/json" \
      -d "{\"query\": \"$query\", \"top_k\": $top_k}")

    retrieval_ms=$(echo $result | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('retrieval_ms',0))")
    generation_ms=$(echo $result | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('generation_ms',0))")
    answer=$(echo $result | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('answer','')[:80])")

    echo "  Q: $query"
    echo "  A: $answer..."
    echo "  retrieval=${retrieval_ms}ms generation=${generation_ms}ms"
    echo ""

    total_ms=$((total_ms + retrieval_ms))
    count=$((count + 1))
  done

  avg_ms=$((total_ms / count))
  echo "  avg retrieval_ms=$avg_ms"
  echo ""
done

echo "======================================"
echo "Check Grafana for faithfulness trends:"
echo "http://localhost:3000"
echo "======================================"
