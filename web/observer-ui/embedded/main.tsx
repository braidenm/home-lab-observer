import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "../src/styles.css";
import "./embedded.css";
import { EmbeddedObserverApp } from "./EmbeddedObserverApp";

const root = document.getElementById("root");
if (!root) throw new Error("Dashboard root is missing");

createRoot(root).render(
  <StrictMode>
    <EmbeddedObserverApp />
  </StrictMode>
);
