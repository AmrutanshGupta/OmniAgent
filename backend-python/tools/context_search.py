import zlib
import os
from langchain.tools import tool
from motor.motor_asyncio import AsyncIOMotorClient

# Get MongoDB URI from environment or default
MONGODB_URI = os.getenv("MONGODB_URI", "mongodb://mongodb:27017")
client = AsyncIOMotorClient(MONGODB_URI)
db = client.omniagent_memory
collection = db.node_results

async def store_node_result(session_id: str, node_id: str, result: str):
    """Compresses and stores the node execution result in MongoDB."""
    compressed_data = zlib.compress(result.encode('utf-8'))
    await collection.update_one(
        {"session_id": session_id, "node_id": node_id},
        {"$set": {"compressed_result": compressed_data}},
        upsert=True
    )

def get_context_search_tool(session_id: str):
    """Returns a tool bound to the specific session_id for querying upstream context."""
    
    @tool
    async def context_search(node_id: str) -> str:
        """
        Retrieves the exact execution output of a previously completed sub-task (node).
        Provide the exact node_id (e.g., 'node_1') to fetch its context.
        """
        doc = await collection.find_one({"session_id": session_id, "node_id": node_id})
        if not doc or "compressed_result" not in doc:
            return f"No context found for node_id: {node_id}"
        
        try:
            decompressed = zlib.decompress(doc["compressed_result"]).decode('utf-8')
            return decompressed
        except Exception as e:
            return f"Error retrieving context: {str(e)}"
            
    return context_search
