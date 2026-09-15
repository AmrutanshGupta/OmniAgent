import { NextResponse } from "next/server";
import { getServerSession } from "next-auth/next";
import { authOptions } from "@/lib/auth";
import mongoose from "mongoose";
import User from "@/models/User";
import { encryptKeys, decryptKeys } from "@/lib/crypto";

const connectDB = async () => {
  if (mongoose.connection.readyState >= 1) return;
  await mongoose.connect(process.env.MONGODB_URI);
};

export async function GET() {
  try {
    const session = await getServerSession(authOptions);
    if (!session) return NextResponse.json({ error: "Unauthorized" }, { status: 401 });

    await connectDB();
    const user = await User.findOne({ email: session.user.email });

    if (!user || !user.keys) {
      return NextResponse.json({ keys: { openai: "", anthropic: "", google: "", groq: "", huggingface: "" } });
    }

    return NextResponse.json({ keys: decryptKeys(user.keys) });
  } catch (error) {
    return NextResponse.json({ error: "Internal Server Error" }, { status: 500 });
  }
}

export async function POST(req) {
  try {
    const session = await getServerSession(authOptions);
    if (!session) return NextResponse.json({ error: "Unauthorized" }, { status: 401 });

    const body = await req.json();
    const keys = body.keys || {};

    await connectDB();

    const encrypted = encryptKeys({
      openai: keys.openai || "",
      anthropic: keys.anthropic || "",
      google: keys.google || "",
      groq: keys.groq || "",
      huggingface: keys.huggingface || "",
    });

    await User.findOneAndUpdate(
      { email: session.user.email },
      {
        $set: {
          "keys.openai": encrypted.openai,
          "keys.anthropic": encrypted.anthropic,
          "keys.google": encrypted.google,
          "keys.groq": encrypted.groq,
          "keys.huggingface": encrypted.huggingface,
        },
      },
      { upsert: true }
    );

    return NextResponse.json({ success: true, message: "Keys encrypted and saved securely." });
  } catch (error) {
    return NextResponse.json({ error: "Internal Server Error" }, { status: 500 });
  }
}