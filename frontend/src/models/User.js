import mongoose from "mongoose";

const UserSchema = new mongoose.Schema({
  email: { type: String, required: true, unique: true },
  keys: {
    openai: { type: String, default: "" },
    anthropic: { type: String, default: "" },
    google: { type: String, default: "" },
    groq: { type: String, default: "" },
    huggingface: { type: String, default: "" },
  },
  sla: { type: Number, default: 50 },
}, { timestamps: true });

export default mongoose.models.User || mongoose.model("User", UserSchema);