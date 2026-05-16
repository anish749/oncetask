import "server-only";
import { Firestore } from "@google-cloud/firestore";

let cached: Firestore | null = null;

// getFirestore returns a singleton Firestore client.
// Auth uses Application Default Credentials. For local dev, run `gcloud auth application-default login`.
// In production, attach a service account (workload identity or GOOGLE_APPLICATION_CREDENTIALS).
//
// Required env: GOOGLE_CLOUD_PROJECT
// Optional env: FIRESTORE_DATABASE_ID (defaults to "(default)" if unset)
export function getFirestore(): Firestore {
  if (cached) return cached;

  const projectId = process.env.GOOGLE_CLOUD_PROJECT;
  if (!projectId) {
    throw new Error(
      "GOOGLE_CLOUD_PROJECT is not set. Configure it in .env.local or your deployment environment.",
    );
  }

  const databaseId = process.env.FIRESTORE_DATABASE_ID;
  cached = new Firestore(
    databaseId ? { projectId, databaseId } : { projectId },
  );
  return cached;
}

// COLLECTION matches oncetask.CollectionOnceTasks in the Go library.
export const COLLECTION = "onceTasks";
