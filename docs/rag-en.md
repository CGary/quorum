# **Master Technical Specification Document: Architecture and Implementation of Production-Grade RAG Systems**

This document constitutes the master technical specification for an Artificial Intelligence agent (or engineering team) to design, implement, evaluate, and deploy an industrial-grade **Retrieval-Augmented Generation (RAG)** system. It integrates theoretical foundations, agentic patterns, vector data structures, and infrastructure optimizations derived from specialized literature and reference production architectures.

---

## **1. Query Taxonomy and Architectural Paradigm Selection**

A robust RAG system does not apply a single rigid strategy to all inputs. It must classify the nature of the incoming query and route it to the appropriate flow.

### **1.1. Query Complexity Taxonomy**

1. **Explicit Factual Queries**: Direct requests for specific data (e.g., *"What is the price of product X?"*). Resolved via **Text-to-SQL** (for dynamic/structured data) or direct vector search (*Top-1/Top-K*).


2. **Implicit Factual Queries**: Require aggregating multiple isolated fragments to infer a unified result. Require **Hybrid Search (Dense + BM25)** and **Reranking**.


3. **Interpretable Justification Queries**: Requests for step-by-step explanations based on regulations or technical manuals. Demand **Query Decomposition** or **Hierarchical RAG (RAPTOR)**.


4. **Hidden Justification / Multihop Queries**: Complex relationships not explicitly declared in the corpus. Require **GraphRAG** or **Cyclic Agentic Flows with LangGraph**.



### **1.2. RAG Architectural Paradigms**

| Paradigm | Components and Mechanisms | Use Case | Advantages / Trade-offs |
| --- | --- | --- | --- |
| **Naive RAG** | Static ingestion, fixed chunking, bi-encoder embedding, simple similarity search, direct context injection into the LLM.

 | Rapid prototyping, simple demos, or very small static knowledge bases.

 | **Advantage**: Trivial implementation.

<br>

<br>**Trade-off**: Vulnerable to noise, loses context, and suffers from hallucinations (factual accuracy ~44%).

 |
| **Advanced RAG** | Pre-retrieval (query transformation/expansion), Hybrid Search (Vectors + BM25), Post-retrieval (Cross-Encoder Reranking, Contextual Retrieval).

 | Enterprise Search, engines for extensive technical documentation.

 | **Advantage**: Raises factual accuracy to >63%, eliminates lexical decontextualization.

<br>

<br>**Trade-off**: Increased inference latency.

 |
| **Modular / Agentic RAG** | State machines (LangGraph), dynamic routing, Self-RAG, Corrective RAG (CRAG) with web search fallback, decoupled storage.

 | Critical support systems, platforms with changing knowledge, and agentic assistants.

 | **Advantage**: High resilience, self-correction of hallucinations, and multi-hop capability.

<br>

<br>**Trade-off**: High orchestration complexity.

 |
| **Text-to-SQL (Structured RAG)** | Relational schema extraction (DDL), translation from prompt to SQL `SELECT`, AST/regex validation, and real-time execution.

 | Queries over inventory, billing, real-time metrics analytics.

 | **Advantage**: Deterministic access to live data.

<br>

<br>**Trade-off**: Risk of SQL injection if permissions are not restricted.

 |

---

## **2. Ingestion, Chunking, and Advanced Indexing Strategies**

### **2.1. Chunking Strategies**

* **Fixed-size Chunking with Overlap**: Division by a fixed token count (e.g., 500 tokens) with overlap (e.g., 10-15% / 50 tokens) to prevent cuts at semantic boundaries.


* **Contextual Retrieval**: Before computing the embedding, a lightweight LLM generates a descriptive contextual header of the source document, which is prepended to each fragment. Reduces retrieval failures by up to 67% by resolving pronominal ambiguities.


* **Late Chunking**: The entire document passes through the encoder Transformer's attention layers first; subsequently, the token segmentation is performed. Each chunk retains global document context without making extra LLM calls.


* **Multi-Representation Indexing (Parent-Document / Proposition Indexing)**:
* **Vector Store**: Stores concise summaries, key phrases, or atomic propositions processed by the LLM and converted into embeddings.


* **Document Store (Key-Value)**: Stores the complete source document or extensive block associated via a `doc_id`.


* **Mechanism**: Vector search is performed on the summary (easier semantic match); upon finding a match, the full document is retrieved from the *Document Store* and delivered to the LLM.




* **Hierarchical Indexing (RAPTOR)**:
1. Base chunks (*leaf chunks*) are clustered using clustering algorithms (e.g., UMAP + GMM).


2. An LLM summarizes each cluster.


3. The process is repeated recursively until a single root summary is obtained.


4. All levels (leaf chunks and cluster summaries) are indexed in the same vector space, effectively answering both detail and global synthesis queries.




* **ColBERT (Late Interaction)**: Generates token-level embeddings instead of a single vector per document. Uses the $MaxSim$ operator to calculate the sum of maximum similarities between query tokens and document tokens, capturing fine-grained semantic nuances.



### **2.2. Asynchronous Incremental Indexing Architecture**

For environments where documents change frequently:

* **State Table (`index_state`)**: Tracks the relationship between the source entity (e.g., `lesson_id` or `doc_id`), its status (`index_pending`, `indexed`, `failed`), and a content hash (`source_hash`).


* **Change Detection**: Any document modification alters the `source_hash`, automatically switching the status to `index_pending`.


* **Asynchronous Worker (AWS Lambda / Celery)**: A background process queries batches with `index_pending` status, executes the chunking and embedding pipeline, updates the vector database, and marks the record as `indexed`.



---

## **3. Vector Representation and High-Dimensional Indexes**

### **3.1. Model Consistency and Operators in PostgreSQL (`pgvector`)**

It is mandatory to use the same embedding model during ingestion and querying. Changing models invalidates the metric space.

In vector-oriented relational databases (such as PostgreSQL with `pgvector`), search operators are defined according to the required geometric distance:

* `<->` : **Euclidean Distance (L2)** ($\sqrt{\sum (x_i - y_i)^2}$).


* `<~>` : **Manhattan Distance (L1)** ($\sum \vert{}x_i - y_i\vert{}$).


* `<=>` : **Cosine Distance** ($1 - \frac{\mathbf{A} \cdot \mathbf{B}}{\Vert{}\mathbf{A}\Vert{} \Vert{}\mathbf{B}\Vert{}}$). Standard choice for text semantic capture regardless of length.



### **3.2. HNSW (Hierarchical Navigable Small World) Algorithm**

Multi-layer graph-based structure for approximate nearest neighbor search (ANN) with logarithmic latency $\mathcal{O}(\log N)$.

#### **Critical Parameters**

1. $M$: Maximum number of outgoing edges per node in each graph layer (Typical range: 16 to 64).


2. $efConstruction$: Size of the priority queue evaluated during index construction. Higher values yield better graph quality at the cost of higher indexing time (Typical range: 64 to 200).


3. $efSearch$: Size of the dynamic candidate list evaluated during query execution. Allows live tuning of the tradeoff between *Recall* and latency (Typical range: 16 to 128).



#### **RAM Consumption Formula for HNSW**

$$\text{Total RAM (Bytes)} = N \cdot \left(4 \cdot d + 8 \cdot M\right) \cdot 1.1$$


Where $N$ is the total number of vectors, $d$ is the dimensionality (e.g., 1536), $M$ is the number of edges per node, and $1.1$ represents a 10% safety margin for graph metadata.

#### **Memory Compression**

* **Scalar Quantization (SQ8)**: Converts 32-bit floats to 8-bit integers, reducing memory size by ~75% with marginal loss of precision.


* **Product Quantization (PQ)**: Divides vectors into sub-vectors and assigns them to centroids, reducing RAM footprint up to 80%.



---

## **4. Query Translation, Routing, and Query Construction (Pre-Retrieval)**

Before querying the database, the user request is processed using refinement techniques:

```
                                 [User Query]
                                      |
                    +-----------------+-----------------+
                    |                                   |
          [Analysis & Translation]                  [Router]
                    |                                   |
 +------------+-----+------------+            +---------+---------+
 |            |                  |            |                   |
[Multi-Query] [HyDE]        [Step-Back]  [Text-to-SQL]    [Vector / BM25]
 |            |                  |     (AST Validation)           |
 +------------+-----+------------+            |                   |
                    |                         +---------+---------+
            [Hybrid Search]                             |
                    |                                   |
             [Reranking Top-N]                          |
                    |                                   |
                    +-----------------+-----------------+
                                      |
                               [LLM Generation]

```

1. **Multi-Query**: An LLM generates 3-5 rewritten variants of the question from different perspectives. Parallel searches are run, and the union of unique documents is consolidated.


2. **RAG Fusion**: Extends *Multi-Query* by applying the **Reciprocal Rank Fusion (RRF)** algorithm to re-rank the consolidated results across all executions.


3. **Query Decomposition**: Breaks down complex questions into logical sub-questions. These are solved sequentially (injecting the previous Question-Answer pair into the next step's context) or in parallel.


4. **Step-Back Prompting**: Generates a more abstract or higher-level question (*step-back question*). Context is retrieved for both the specific query and the general concept.


5. **HyDE (Hypothetical Document Embeddings)**: The LLM generates a fictitious "hypothetical" answer. The embedding of this synthetic answer is used to query the vector space, retrieving real documents that share its semantic structure.


6. **Logical Routing & Semantic Routing**:
* *Logical*: An LLM analyzes intent and, via *Structured Outputs* (Pydantic / Function Calling), decides which source to query (e.g., SQL, Vector Store A, Vector Store B).


* *Semantic*: Calculates the query embedding and measures cosine similarity against a set of pre-vectorized prompts or database descriptions.




7. **Query Construction (Text-to-Metadata Filters)**: Translates natural language requests (*"AI videos published after 2024"*) into structured filter objects applied over vector store metadata.


8. **Safe Text-to-SQL**:
* LLM Temperature set to `0`.


* Validation passes (via AST or Regular Expressions) to prohibit destructive instructions (`INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`).


* Mandatory limit injection on query execution (`LIMIT 50`).





---

## **5. Hybrid Retrieval and Post-Processing (Retrieval & Post-Retrieval)**

### **5.1. Hybrid Search with Reciprocal Rank Fusion (RRF)**

Combines the strengths of semantic vector search (bi-encoders for abstract concepts) with sparse lexical search (BM25 for exact codes, proper nouns, and identifiers).

Since BM25 scores and cosine similarity scores are on different scales, RRF merges lists based exclusively on the ordinal rank of documents:

$$RRF\_Score(d \in D) = \sum_{m \in M} \frac{1}{k + r_m(d)}$$

* $M$: Set of retrieval systems (e.g., Vectorial and BM25).


* $r_m(d)$: Rank or position order of document $d$ in search system $m$.


* $k$: Smoothing constant that mitigates the influence of top positions. Industry standard is $k = 60$.



### **5.2. Re-ranking (Cross-Encoder Reranking)**

The top $K$ candidates returned by RRF (e.g., Top-50) are processed through a **Cross-Encoder** model. Unlike Bi-Encoders, Cross-Encoders evaluate the query and document simultaneously across cross-attention layers, generating a highly accurate relevance score to select the final Top-$N$ (e.g., Top-5 or Top-10) passed to the LLM.

---

## **6. Agentic Orchestration and Inference Flows (LangGraph State Machines)**

Modern paradigms replace static linear chains with deterministic state machines guided by directed graphs (LangGraph).

```
                 +-------------------+
                 | User Input        |
                 +---------+---------+
                           |
                           v
                 +-------------------+
                 | Router / Analysis |
                 +----+---------+----+
                      |         |
      +---------------+         +---------------+
      | (SQL Query)                             | (Vector Query)
      v                                         v
+------------+                            +-----------+
| Text-to-SQL|                            | Retrieval |
+-----+------+                            +-----+-----+
      |                                         |
      |                                         v
      |                                  +--------------+
      |                                  | Reranking    |
      |                                  +------+-------+
      |                                         |
      |                                         v
      |                                  +--------------+
      |                                  | Doc Grader   |
      |                                  +------+-------+
      |                                         |
      |                       +-----------------+-----------------+
      |                       | (Insufficient)                    | (Sufficient)
      |                       v                                   v
      |               +---------------+                  +------------------+
      |               | Corrective/Web|                  | Generation (LLM) |
      |               +-------+-------+                  +--------+---------+
      |                       |                                   |
      |                       +-----------------+-----------------+
      |                                         |
      v                                         v
+---------------+                        +--------------+
| Validation    |                        | Self-RAG     |
| Result        |                        | Verification |
+-----+---------+                        +------+-------+
      |                                         |
      +-------------------+---------------------+
                          |
                          v
                +-------------------+
                | Final Output      |
                +-------------------+

```

### **6.1. State Definition (`GraphState`)**

The state is a strongly-typed object (dict or dataclass) propagated and mutated at each node:

* `query` *(str)*: Original or rewritten query.


* `documents` *(List[Document])*: List of retrieved/filtered chunks.


* `generation` *(str)*: Temporary or final generated answer.


* `search_needed` *(bool)*: Flag to trigger external web search.


* `retry_count` *(int)*: Counter to prevent infinite loops.



### **6.2. Integrated Agentic Patterns**

* **Corrective RAG (CRAG)**: An evaluator node (*Grader*) checks retrieved documents. If context is irrelevant or insufficient, the graph follows a conditional edge to a query rewriter node and executes a web search (e.g., Tavily API) to supplement context.


* **Self-RAG**: The LLM outputs internal control evaluations:


1. `IS_REL`: Is the retrieved document relevant?


2. `IS_SUP`: Is the response fully supported by the context? (Hallucination detection).


3. `IS_USE`: Does the response solve the user's original query?
If the response fails hallucination or relevance checks, the graph restarts the retrieval or rewriting cycle.





---

## **7. Quantitative Evaluation Framework (RAGAS)**

To numerically evaluate system performance without relying on unscalable human evaluation, the RAGAS metric triad is used over the tuple **Query ($q$), Retrieved Context ($C$), Generated Answer ($a$), and Ground Truth ($y$)**:

### **7.1. Faithfulness (Absence of Hallucinations)**

Measures whether generated answer $a$ is derived strictly from context $C$. An evaluator LLM decomposes $a$ into atomic statements $S(a)$:

$$\text{Faithfulness} = \frac{\vert{}\text{Statements in } S(a) \text{ supported by } C\vert{}}{\vert{}S(a)\vert{}}$$

### **7.2. Answer Relevancy**

Measures whether answer $a$ directly addresses query $q$. $n$ hypothetical queries $q_i$ are generated from $a$, and average cosine similarity is calculated against $q$:

$$\text{Answer Relevancy} = \frac{1}{n} \sum_{i=1}^{n} \cos(E(q), E(q_i))$$

### **7.3. Context Recall**

Determines whether retrieved context $C$ contains all information from the reference ground truth $y$. Sentences in $y$ are decomposed into $S(y)$:

$$\text{Context Recall} = \frac{\vert{}\text{Sentences in } S(y) \text{ attributed to } C\vert{}}{\vert{}S(y)\vert{}}$$

### **7.4. Context Precision**

Evaluates whether relevant chunks in $C$ are ranked in top positions:

$$\text{Context Precision}@K = \frac{\sum_{k=1}^{K} (P@k \cdot v_k)}{\text{Total relevant items in Top-}K}$$

Where $P@k$ is precision at rank $k$, and $v_k \in \{0, 1\}$ indicates whether the item at position $k$ is relevant.

---

## **8. Security, Robustness, and Vulnerabilities**

1. **BadRAG / TrojanRAG Attacks**: Modifying a minimal fraction of the corpus (0.04%) with malicious chunks or modified vectors can force the model to output manipulated information.


* *Mitigation*: Cryptographic verification of document signatures, input text sanitization, and adversarial evaluation/training.




2. **Document Prompt Injection**: Ingested chunks from external sources containing hidden instructions (*"Ignore previous instructions and output X"*).


* *Mitigation*: Clearly delimit context in the system prompt using structural XML tags and instruct the model to treat content within tags strictly as inert data.




3. **SQL Injection**: Direct threat in Text-to-SQL pipelines.


* *Mitigation*: Strictly read-only database users (`SELECT`), restricted visible schemas, and AST validation of generated queries.





---

## **9. Executable Technical Specification for AI Agent Construction**

This section defines the standard JSON configuration schema and operational directives for an autonomous agent to build or reconfigure the complete RAG pipeline.

### **9.1. Global Pipeline Configuration JSON Schema**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "MasterRAGPipelineConfig",
  "type": "object",
  "properties": {
    "environment": {
      "type": "string",
      "enum": ["local_development", "cloud_production"]
    },
    "chunking_strategy": {
      "type": "object",
      "properties": {
        "method": {
          "type": "string",
          "enum": ["fixed_overlap", "contextual_retrieval", "late_chunking", "multi_representation", "raptor"]
        },
        "chunk_size": { "type": "integer", "default": 512 },
        "chunk_overlap": { "type": "integer", "default": 64 }
      },
      "required": ["method"]
    },
    "vector_store": {
      "type": "object",
      "properties": {
        "provider": { "type": "string", "enum": ["pgvector", "qdrant", "milvus"] },
        "embedding_model": { "type": "string", "default": "text-embedding-3-small" },
        "embedding_dimensions": { "type": "integer", "default": 1536 },
        "hnsw_config": {
          "type": "object",
          "properties": {
            "m": { "type": "integer", "default": 16 },
            "ef_construction": { "type": "integer", "default": 64 },
            "ef_search": { "type": "integer", "default": 32 }
          },
          "required": ["m", "ef_construction", "ef_search"]
        }
      },
      "required": ["provider", "embedding_model", "embedding_dimensions", "hnsw_config"]
    },
    "hybrid_search": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean", "default": true },
        "sparse_provider": { "type": "string", "enum": ["bm25_opensearch", "pg_trgm_bm25"] },
        "k_dense": { "type": "integer", "default": 50 },
        "k_sparse": { "type": "integer", "default": 50 },
        "rrf_k_constant": { "type": "integer", "default": 60 }
      },
      "required": ["enabled", "k_dense", "k_sparse", "rrf_k_constant"]
    },
    "post_processing": {
      "type": "object",
      "properties": {
        "reranker_enabled": { "type": "boolean", "default": true },
        "reranker_model": { "type": "string", "default": "cross-encoder/ms-marco-MiniLM-L-6-v2" },
        "top_n_final": { "type": "integer", "default": 5 }
      },
      "required": ["reranker_enabled", "reranker_model", "top_n_final"]
    },
    "orchestration": {
      "type": "object",
      "properties": {
        "framework": { "type": "string", "enum": ["langgraph"] },
        "agentic_patterns": {
          "type": "object",
          "properties": {
            "crag_web_fallback": { "type": "boolean", "default": true },
            "self_rag_hallucination_check": { "type": "boolean", "default": true },
            "text_to_sql_enabled": { "type": "boolean", "default": true }
          },
          "required": ["crag_web_fallback", "self_rag_hallucination_check", "text_to_sql_enabled"]
        }
      },
      "required": ["framework", "agentic_patterns"]
    },
    "evaluation_thresholds": {
      "type": "object",
      "properties": {
        "min_faithfulness": { "type": "number", "default": 0.85 },
        "min_answer_relevancy": { "type": "number", "default": 0.80 },
        "min_context_recall": { "type": "number", "default": 0.80 }
      },
      "required": ["min_faithfulness", "min_answer_relevancy", "min_context_recall"]
    }
  },
  "required": [
    "environment",
    "chunking_strategy",
    "vector_store",
    "hybrid_search",
    "post_processing",
    "orchestration",
    "evaluation_thresholds"
  ]
}

```

---

### **9.2. Step-by-Step Construction Protocol for the AI Agent**

1. **Step 1: Environment and Data Schema Initialization**:
* Read `MasterRAGPipelineConfig`.


* If `environment == "local_development"`, instantiate Docker containers for the vector store (e.g., Qdrant or PostgreSQL with `pgvector`) and Ollama/vLLM for local inference.


* Create the incremental tracking table `index_state` with fields `doc_id`, `source_hash`, and `status` (`index_pending`, `indexed`, `failed`).




2. **Step 2: Ingestion and Indexing Pipeline**:
* Load document corpus.


* Execute strategy defined in `chunking_strategy` (e.g., if `contextual_retrieval`, invoke LLM to prepend context summary before chunking).


* Generate embeddings using `embedding_model`.


* Configure HNSW index on the vector store with assigned $M$, $efConstruction$, and $efSearch$ parameters.




3. **Step 3: Orchestration Graph Construction (LangGraph)**:
* Define `GraphState` (`query`, `documents`, `generation`, `search_needed`, `retry_count`).


* **Router Node**: Implement classification logic to determine if query is explicit factual for **Text-to-SQL** (with read-only AST validation and `LIMIT 50`) or requires **Vector/Hybrid Search**.


* **Hybrid Retrieval Node**: Run dense and sparse (BM25) searches in parallel, applying RRF formula with $k=60$ to consolidate candidates.


* **Reranker Node**: Apply Cross-Encoder over RRF candidates to select `top_n_final`.


* **Evaluator Node (CRAG)**: Check context relevance. If documents do not pass filter, set `search_needed = True` and route to Web Search node.


* **Generator and Verifier Node (Self-RAG)**: Produce answer and invoke hallucination evaluation checks. If unsupported information is detected and `retry_count < max`, rewrite query and retry.




4. **Step 4: Quantitative Verification (RAGAS Benchmarking)**:
* Run test dataset (*Ground Truth*) against RAGAS evaluation API.


* Verify results meet configured thresholds:
* $\text{Faithfulness} \ge 0.85$

* $\text{Answer Relevancy} \ge 0.80$

* $\text{Context Recall} \ge 0.80$



* If metrics fall below threshold, adjust hyperparameters (e.g., increase $efSearch$, adjust RRF $k$ constant, or switch Cross-Encoder model).
