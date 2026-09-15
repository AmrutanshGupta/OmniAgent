import GithubProvider from "next-auth/providers/github";
import jwt from "jsonwebtoken";
import mongoose from "mongoose";
import User from "@/models/User";

const connectDB = async () => {
  if (mongoose.connection.readyState >= 1) return;
  await mongoose.connect(process.env.MONGODB_URI, {
    family: 4,
    serverSelectionTimeoutMS: 5000,
  });
};

export const authOptions = {
  providers: [
    GithubProvider({
      clientId: process.env.GITHUB_ID,
      clientSecret: process.env.GITHUB_SECRET,
    }),
  ],
  session: { strategy: "jwt" },
  callbacks: {
    async signIn({ user }) {
      await connectDB();
      const existingUser = await User.findOne({ email: user.email });
      if (!existingUser) {
        await User.create({ email: user.email });
        console.log(`New user registered in MongoDB: ${user.email}`);
      }
      return true;
    },
    async session({ session, token }) {
      session.user.email = token.email;
      const rawToken = jwt.sign(token, process.env.NEXTAUTH_SECRET, { algorithm: "HS256" });
      session.token = rawToken;
      return session;
    },
  },
  jwt: {
    encode: async ({ secret, token }) => {
      return jwt.sign(token, secret, { algorithm: "HS256" });
    },
    decode: async ({ secret, token }) => {
      return jwt.verify(token, secret, { algorithms: ["HS256"] });
    },
  },
};