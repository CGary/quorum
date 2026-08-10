# **Documento Maestro de Especificación Técnica: Arquitectura e Implementación de Sistemas RAG de Nivel de Producción**

Este documento constituye la especificación técnica maestra para que un agente de Inteligencia Artificial (o un equipo de ingeniería) diseñe, implemente, evalúe y despliegue un sistema de **Generación Aumentada por Recuperación (RAG)** de grado industrial. Integra los fundamentos teóricos, los patrones agénticos, las estructuras de datos vectoriales y las optimizaciones de infraestructura derivadas de la literatura especializada y de las arquitecturas de producción de referencia.

---

## **1. Taxonomía de Consultas y Selección del Paradigma Arquitectónico**

Un sistema RAG robusto no aplica una única estrategia rígida a todas las entradas. Debe clasificar la naturaleza de la consulta y derivarla al flujo adecuado.

### **1.1. Taxonomía de Complejidad de Consultas**

1. **Consultas de Hechos Explícitos**: Solicitud puntual de datos concretos (ej. *"¿Cuál es el precio del producto X?"*). Se resuelve vía **Text-to-SQL** (si son datos dinámicos/estructurados) o búsqueda vectorial directa (*Top-1/Top-K*).


2. **Consultas de Hechos Implícitos**: Exigen agregar múltiples fragmentos aislados para inferir un resultado unificado. Requiere **Búsqueda Híbrida (Dense + BM25)** y **Reranking**.


3. **Consultas de Justificación Interpretable**: Peticiones de explicaciones paso a paso basadas en normativas o manuales técnicos. Demanda **Descomposición de Consultas** o **RAG Jerárquico (RAPTOR)**.


4. **Consultas de Justificación Oculta / Multihop**: Relaciones complejas no declaradas explícitamente en el corpus. Requiere **GraphRAG** o **Flujos Agénticos Cíclicos con LangGraph**.



### **1.2. Paradigmas de Arquitectura RAG**

| Paradigma | Componentes y Mecanismos | Caso de Uso | Ventajas / Limitaciones |
| --- | --- | --- | --- |
| **Naive RAG** | Ingesta estática, *chunking* fijo, embedding bi-encoder, búsqueda por similitud simple, contexto inyectado directo al LLM.

 | Prototipado rápido, demos simples o bases de conocimiento estáticas muy pequeñas.

 | **Ventaja**: Implementación trivial.

<br>

<br>**Desafío**: Vulnerable al ruido, pierde contexto y sufre alucinaciones (precisión fáctica ~44%).

 |
| **Advanced RAG** | Pre-retrieval (transformación/ampliación de queries), Búsqueda Híbrida (Vectores + BM25), Post-retrieval (Cross-Encoder Reranking, Contextual Retrieval).

 | Enterprise Search, motores de búsqueda en documentación técnica extensa.

 | **Ventaja**: Eleva precisión fáctica a >63%, elimina descontextualización léxica.

<br>

<br>**Desafío**: Incremento en la latencia de inferencia.

 |
| **Modular / Agentic RAG** | Máquinas de estado (LangGraph), enrutamiento dinámico, Self-RAG, Corrective RAG (CRAG) con *fallback* a web search, almacenamiento desacoplado.

 | Sistemas de soporte crítico, plataformas de conocimiento cambiante y asistentes agénticos.

 | **Ventaja**: Alta resiliencia, auto-corrección de alucinaciones y capacidad multi-hop.

<br>

<br>**Desafío**: Alta complejidad de orquestación.

 |
| **Text-to-SQL (RAG Estructurado)** | Extracción del esquema relacional (DDL), traducción del prompt a SQL `SELECT`, validación AST/regex y ejecución en tiempo real.

 | Consultas sobre inventarios, facturación, analítica de métricas en tiempo real. | **Ventaja**: Acceso determinista a datos en vivo.

<br>

<br>**Desafío**: Riesgo de inyección SQL si no se restringen permisos.

 |

---

## **2. Ingesta, Chunking y Estrategias de Indexación Avanzada**

### **2.1. Estrategias de Segmentación (Chunking)**

* **Fragmentación Fija con Overlap**: División por cantidad fija de tokens (ej. 500 tokens) con superposición (ej. 10-15% / 50 tokens) para evitar cortes en las fronteras semánticas.


* **Contextual Retrieval**: Antes de calcular el embedding, un LLM liviano genera un encabezado descriptivo del documento de origen que se antepone a cada fragmento. Reduce las fallas de recuperación hasta en un 67% al resolver ambigüedades pronominales.


* **Late Chunking**: El documento completo pasa primero por las capas de atención del modelo Transformer codificador; posteriormente se realiza el corte de los tokens. Cada fragmento retiene el contexto global del texto sin realizar llamadas extra al LLM.


* **Multi-Representation Indexing (Parent-Document / Proposition Indexing)**:
* **Vector Store**: Almacena resúmenes concisos, frases clave o proposiciones atómicas procesadas por el LLM y convertidas en embeddings.


* **Document Store (Key-Value)**: Guarda el documento completo o el bloque extenso de origen asociado mediante un `doc_id`.


* **Mecanismo**: La búsqueda vectorial se realiza sobre el resumen (más fácil de emparejar semánticamente); al encontrar coincidencia, se extrae el documento completo del *Document Store* y se entrega al LLM.




* **Indexación Jerárquica (RAPTOR)**:
1. Los fragmentos base (*leaf chunks*) se agrupan mediante algoritmos de *clustering* (ej. UMAP + GMM).


2. Un LLM resume cada clúster.


3. El proceso se repite de forma recursiva hasta obtener una única raíz resumen.


4. Todos los niveles (fragmentos base y resúmenes de clúster) se indexan en el mismo espacio vectorial, respondiendo eficazmente tanto preguntas de detalle como de síntesis global.




* **ColBERT (Late Interaction)**: Genera embeddings a nivel de token en lugar de un único vector por documento. Utiliza el operador $MaxSim$ para calcular la suma de las máximas similitudes entre los tokens de la consulta y los del documento, capturando matices semánticos finos.



### **2.2. Arquitectura de Indexación Incremental Asíncrona**

Para entornos donde los documentos cambian con frecuencia:

* **Tabla de Estado (`index_state`)**: Registra la relación entre la entidad de origen (ej. `lesson_id` o `doc_id`), su estado (`index_pending`, `indexed`, `failed`) y un hash de su contenido (`source_hash`).


* **Detección de Cambios**: Cualquier modificación en el documento altera el `source_hash`, conmutando el estado automáticamente a `index_pending`.


* **Worker Asíncrono (AWS Lambda / Celery)**: Un proceso en segundo plano consulta lotes con estado `index_pending`, ejecuta el pipeline de *chunking* y *embedding*, actualiza la base vectorial y marca el registro como `indexed`.



---

## **3. Representación Vectorial e Índices de Alta Dimensión**

### **3.1. Consistencia de Modelos y Operadores en PostgreSQL (`pgvector`)**

Es obligatorio utilizar el mismo modelo de embeddings tanto en la ingesta como en la consulta. Cambiar de modelo invalida el espacio métrico.

En bases de datos relacionales orientadas a vectores (como PostgreSQL con `pgvector`), los operadores de búsqueda se definen según la distancia geométrica requerida:

* `<->` : **Distancia Euclídea (L2)** ($\sqrt{\sum (x_i - y_i)^2}$).


* `<~>` : **Distancia Manhattan (L1)** ($\sum \vert{}x_i - y_i\vert{}$).


* `<=>` : **Distancia Coseno** ($1 - \frac{\mathbf{A} \cdot \mathbf{B}}{\Vert{}\mathbf{A}\Vert{} \Vert{}\mathbf{B}\Vert{}}$). Es la opción estándar para captura semántica de texto independientemente de la longitud.



### **3.2. Algoritmo HNSW (Hierarchical Navigable Small World)**

Estructura basada en grafos multicapa para búsqueda de vecinos más cercanos aproximados (ANN) con latencia logarítmica $\mathcal{O}(\log N)$.

#### **Parámetros Críticos**

1. $M$: Número máximo de conexiones o aristas de salida por nodo en cada capa del grafo (Rango típico: 16 a 64).


2. $efConstruction$: Tamaño de la cola prioritaria de exploración durante la construcción del índice. A mayor valor, mayor calidad del grafo y mayor tiempo de indexación (Rango típico: 64 a 200).


3. $efSearch$: Tamaño de la lista de candidatos evaluados dinámicamente en tiempo de consulta. Permite ajustar en vivo el compromiso entre *Recall* y latencia (Rango típico: 16 a 128).



#### **Cálculo de Consumo de Memoria RAM para HNSW**

$$\text{RAM Total (Bytes)} = N \cdot \left(4 \cdot d + 8 \cdot M\right) \cdot 1.1$$


Donde $N$ es el total de vectores, $d$ es la dimensionalidad (ej. 1536), $M$ es el número de aristas por nodo, y $1.1$ representa un 10% de margen para metadatos del grafo.

#### **Compresión de Memoria**

* **Scalar Quantization (SQ8)**: Convierte flotantes de 32 bits a enteros de 8 bits, reduciendo el tamaño en memoria ~75% con pérdida marginal de precisión.


* **Product Quantization (PQ)**: Divide el vector en sub-vectores y los asigna a centroides, reduciendo la huella en RAM hasta un 80%.



---

## **4. Traducción, Enrutamiento y Construcción de Consultas (Pre-Retrieval)**

Antes de consultar la base de datos, la petición del usuario se procesa mediante técnicas de refinamiento:

```
                                [Consulta de Usuario]
                                          |
                        +-----------------+-----------------+
                        |                                   |
              [Análisis y Traducción]                    [Enrutador]
                        |                                   |
     +------------+-----+------------+            +---------+---------+
     |            |                  |            |                   |
 [Multi-Query] [HyDE]          [Step-Back]   [Text-to-SQL]    [Vector / BM25]
     |            |                  |     (Validación AST)           |
     +------------+-----+------------+            |                   |
                        |                         +---------+---------+
              [Búsqueda Híbrida]                            |
                        |                                   |
               [Reranking Top-N]                            |
                        |                                   |
                        +-----------------+-----------------+
                                          |
                                 [Generación LLM]

```

1. **Multi-Query**: Un LLM genera 3-5 variantes reescritas de la pregunta desde distintas perspectivas. Se ejecutan búsquedas paralelas y se consolida la unión de documentos únicos.


2. **RAG Fusion**: Extiende *Multi-Query* aplicando el algoritmo **Reciprocal Rank Fusion (RRF)** para reordenar los resultados consolidados de todas las ejecuciones.


3. **Query Decomposition**: Descompone preguntas complejas en sub-preguntas lógicas. Se resuelven de manera secuencial (inyectando el par Pregunta-Respuesta previo en el contexto del siguiente paso) o en paralelo.


4. **Step-Back Prompting**: Genera una pregunta más abstracta o de nivel superior (*step-back question*). Se recupera contexto tanto para la duda específica como para el concepto general.


5. **HyDE (Hypothetical Document Embeddings)**: El LLM genera una respuesta "hipotética" ficticia. El embedding de esta respuesta sintética se utiliza para consultar el espacio vectorial, buscando documentos reales que compartan su estructura semántica.


6. **Logical Routing & Semantic Routing**:
* *Logical*: Un LLM analiza la intención y, mediante *Structured Outputs* (Pydantic / Function Calling), decide qué fuente consultar (ej. SQL, Vector Store A, Vector Store B).


* *Semantic*: Se calcula el embedding de la pregunta y se mide la similitud coseno contra un conjunto de *prompts* o descripciones de bases de datos pre-vectorizadas.




7. **Query Construction (Text-to-Metadata Filters)**: Traduce peticiones en lenguaje natural (*"videos sobre AI publicados después de 2024"*) a objetos de filtro estructurados que se aplican sobre los metadatos de la base vectorial.


8. **Text-to-SQL Seguro**:
* Temperatura del LLM en `0`.


* Pases de validación (vía AST o Expresiones Regulares) para prohibir instrucciones destructivas (`INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`).


* Inyección obligatoria de límites en la consulta (`LIMIT 50`).





---

## **5. Recuperación Híbrida y Post-Procesamiento (Retrieval & Post-Retrieval)**

### **5.1. Búsqueda Híbrida con Reciprocal Rank Fusion (RRF)**

Combina las fortalezas de la búsqueda semántica vectorial (bi-encoders para conceptos abstractos) con la búsqueda léxica esparsa (BM25 para códigos, nombres propios e identificadores exactos).

Dado que las puntuaciones de BM25 y de la similitud coseno no están en la misma escala, RRF fusiona las listas basándose exclusivamente en la posición ordinal (rango) de los documentos:

$$RRF\_Score(d \in D) = \sum_{m \in M} \frac{1}{k + r_m(d)}$$

* $M$: Conjunto de sistemas de búsqueda (ej. Vectorial y BM25).


* $r_m(d)$: Rango u orden de posición del documento $d$ en el sistema de búsqueda $m$.


* $k$: Constante de suavizado que atenúa la influencia de los primeros lugares absolutos. El estándar en la industria es $k = 60$.



### **5.2. Re-ordenamiento (Reranking con Cross-Encoder)**

Los primeros $K$ candidatos devueltos por RRF (ej. Top-50) se procesan a través de un modelo **Cross-Encoder**. A diferencia de los Bi-Encoders, el Cross-Encoder evalúa la consulta y el documento de forma simultánea en sus capas de atención cruzada, generando una puntuación de relevancia altamente precisa para seleccionar los Top-$N$ finales (ej. Top-5 o Top-10) que se enviarán al LLM.

---

## **6. Orquestación Agéntica y Flujos de Inferencia (LangGraph State Machines)**

El paradigma moderno reemplaza las cadenas lineales estáticas por máquinas de estado deterministas guiadas por grafos dirigidos (LangGraph).

```
                 +-------------------+
                 | Entrada Usuario   |
                 +---------+---------+
                           |
                           v
                 +-------------------+
                 | Router / Análisis |
                 +----+---------+----+
                      |         |
      +---------------+         +---------------+
      | (Consulta SQL)                          | (Consulta Vectorial)
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
      |                                  | Grader Doc   |
      |                                  +------+-------+
      |                                         |
      |                       +-----------------+-----------------+
      |                       | (Insuficiente)                    | (Suficiente)
      |                       v                                   v
      |               +---------------+                  +------------------+
      |               | Corrective/Web|                  | Generación (LLM) |
      |               +-------+-------+                  +--------+---------+
      |                       |                                   |
      |                       +-----------------+-----------------+
      |                                         |
      v                                         v
+---------------+                        +--------------+
| Validación    |                        | Self-RAG     |
| Resultado     |                        | Verification |
+-----+---------+                        +------+-------+
      |                                         |
      +-------------------+---------------------+
                          |
                          v
                +-------------------+
                | Salida Final      |
                +-------------------+

```

### **6.1. Definición del Estado (`GraphState`)**

El estado es un objeto fuertemente tipado (dict o dataclass) que se propaga y modifica en cada nodo:

* `query` *(str)*: Consulta original o reescrita.


* `documents` *(List[Document])*: Lista de fragmentos recuperados/filtrados.


* `generation` *(str)*: Respuesta temporal o final generada.


* `search_needed` *(bool)*: Bandera para activar búsqueda web externa.


* `retry_count` *(int)*: Contador de reintentos para evitar bucles infinitos.



### **6.2. Patrones Agénticos Integrados**

* **Corrective RAG (CRAG)**: Un nodo evaluador (*Grader*) analiza los documentos recuperados. Si el contexto es irrelevante o insuficiente, el grafo toma una arista condicional hacia un nodo de reescritura de consulta y ejecuta una búsqueda web (ej. Tavily API) para complementar el contexto.


* **Self-RAG**: El LLM genera tokens o evaluaciones de control interno:


1. `IS_REL`: ¿Es relevante el documento recuperado?


2. `IS_SUP`: ¿Está la respuesta totalmente respaldada por el contexto? (Detección de alucinaciones).


3. `IS_USE`: ¿Resuelve la respuesta la pregunta original del usuario?
Si la respuesta falla la prueba de alucinación o relevancia, el grafo reinicia el ciclo de búsqueda o reescritura.





---

## **7. Marco de Evaluación Cuantitativa (RAGAS)**

Para medir numéricamente la efectividad del sistema sin depender de evaluaciones humanas no escalables, se utiliza la tríada de métricas del marco RAGAS sobre la tupla **Consulta ($q$), Contexto Recuperado ($C$), Respuesta Generada ($a$) y Ground Truth ($y$)**:

### **7.1. Faithfulness (Fidelidad / Ausencia de Alucinaciones)**

Mide si la respuesta generada $a$ se deriva exclusivamente del contexto $C$. Un LLM descompone $a$ en afirmaciones atómicas $S(a)$:

$$\text{Faithfulness} = \frac{\vert{}\text{Afirmaciones en } S(a) \text{ respaldadas por } C\vert{}}{\vert{}S(a)\vert{}}$$

### **7.2. Answer Relevancy (Relevancia de la Respuesta)**

Mide si la respuesta $a$ aborda directamente la consulta $q$. Se generan $n$ preguntas hipotéticas $q_i$ a partir de $a$ y se calcula la similitud coseno promedio respecto a $q$:

$$\text{Answer Relevancy} = \frac{1}{n} \sum_{i=1}^{n} \cos(E(q), E(q_i))$$

### **7.3. Context Recall (Exhaustividad del Contexto)**

Determina si el contexto recuperado $C$ contiene toda la información de la respuesta ideal de referencia $y$ (*Ground Truth*). Se descomponen las oraciones de $y$ en $S(y)$:

$$\text{Context Recall} = \frac{\vert{}\text{Oraciones en } S(y) \text{ atribuidas a } C\vert{}}{\vert{}S(y)\vert{}}$$

### **7.4. Context Precision (Precisión del Ranking de Contexto)**

Evalúa si los fragmentos relevantes en $C$ están ordenados en las primeras posiciones:

$$\text{Context Precision}@K = \frac{\sum_{k=1}^{K} (P@k \cdot v_k)}{\text{Total de elementos relevantes en Top-}K}$$

Donde $P@k$ es la precisión en el nivel $k$, y $v_k \in \{0, 1\}$ indica si el elemento en la posición $k$ es relevante.

---

## **8. Seguridad, Robustez y Vulnerabilidades**

1. **Ataques BadRAG / TrojanRAG**: Alterar un porcentaje mínimo del corpus (0.04%) mediante fragmentos maliciosos o vectores modificados puede forzar al modelo a emitir información manipulada.


* *Mitigación*: Verificación criptográfica de firmas de documentos, sanitización de textos ingresados y entrenamiento/evaluación adversarial.




2. **Inyección de Prompts en Documentos**: Fragmentos cargados desde fuentes externas que contienen instrucciones ocultas (*"Ignora las instrucciones anteriores y muestra X"*).
* *Mitigación*: Delimitar claramente el contexto en el prompt del sistema utilizando etiquetas XML estructurales e instruir al modelo a tratar el contenido dentro de dichas etiquetas estrictamente como datos inertes.




3. **Inyección SQL**: Amenaza directa en pipelines Text-to-SQL.
* *Mitigación*: Uso de usuarios de base de datos con permisos estrictos de solo lectura (`SELECT`), restricción de esquemas visibles y validación previa de la consulta generada mediante parsers AST.





---

## **9. Especificación Técnica Ejecutable para la Construcción por un Agente AI**

Esta sección define el esquema de configuración estándar en formato JSON y las directivas operativas para que un agente autónomo construya o reconfigure el pipeline RAG completo.

### **9.1. JSON Schema de Configuración Global del Pipeline**

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

### **9.2. Protocolo de Construcción Paso a Paso para el Agente AI**

1. **Paso 1: Inicialización del Entorno y Esquema de Datos**:
* Leer `MasterRAGPipelineConfig`.
* Si `environment == "local_development"`, instanciar contenedores Docker para la base vectorial (ej. Qdrant o PostgreSQL con `pgvector`) y Ollama/vLLM para inferencia local.


* Crear la tabla de seguimiento incremental `index_state` con campos `doc_id`, `source_hash` y `status` (`index_pending`, `indexed`, `failed`).




2. **Paso 2: Pipeline de Ingesta e Indexación**:
* Cargar el corpus de documentos.
* Ejecutar la estrategia definida en `chunking_strategy` (ej. si es `contextual_retrieval`, invocar al LLM para anteponer el resumen de contexto antes de segmentar).


* Generar embeddings con el modelo especificado (`embedding_model`).


* Configurar el índice HNSW en la base vectorial asignando los parámetros $M$, $efConstruction$ y $efSearch$.




3. **Paso 3: Construcción del Grafo de Orquestación (LangGraph)**:
* Definir el estado `GraphState` (`query`, `documents`, `generation`, `search_needed`, `retry_count`).
* **Nodo Router**: Implementar la lógica para clasificar si la consulta es de hechos explícitos para **Text-to-SQL** (con validación AST de solo lectura y `LIMIT 50`) o si requiere **Búsqueda Vectorial/Híbrida**.


* **Nodo Retrieval Híbrido**: Ejecutar en paralelo la búsqueda densa y la búsqueda esparsa (BM25), aplicando la fórmula RRF con $k=60$ para consolidar los candidatos.


* **Nodo Reranker**: Aplicar el Cross-Encoder sobre los candidatos RRF para seleccionar el `top_n_final`.


* **Nodo Evaluador (CRAG)**: Verificar la relevancia del contexto. Si los documentos no superan el filtro, activar `search_needed = True` y derivar al nodo de Búsqueda Web.


* **Nodo Generador y Verificador (Self-RAG)**: Producir la respuesta e invocar las pruebas de evaluación de alucinación. Si se detecta información no respaldada y `retry_count < max`, reescribir la consulta y reintentar.




4. **Paso 4: Verificación Cuantitativa (RAGAS Benchmarking)**:
* Correr un conjunto de datos de prueba (*Ground Truth*) sobre la API de evaluación RAGAS.


* Validar que los resultados cumplan con los umbrales configurados:
* $\text{Faithfulness} \ge 0.85$

* $\text{Answer Relevancy} \ge 0.80$

* $\text{Context Recall} \ge 0.80$



* Si las métricas se encuentran por debajo del umbral, ajustar hiperparámetros (p. ej., aumentar $efSearch$, incrementar la constante $k$ de RRF o cambiar el Cross-Encoder).
