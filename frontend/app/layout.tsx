import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Ezra Clone - Agent Development Environment",
  description: "Visualize and edit your AI agent's memory",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}

