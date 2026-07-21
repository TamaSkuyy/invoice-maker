import { describe, it, expect } from "vitest";
import { ApiError } from "./api";

describe("ApiError", () => {
  it("should create an error with status and message", () => {
    const error = new ApiError(404, "Not Found");

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(ApiError);
    expect(error.status).toBe(404);
    expect(error.message).toBe("Not Found");
    expect(error.name).toBe("ApiError");
  });

  it("should handle 500 server error", () => {
    const error = new ApiError(500, "Internal Server Error");

    expect(error.status).toBe(500);
    expect(error.message).toBe("Internal Server Error");
  });

  it("should handle 422 validation error", () => {
    const error = new ApiError(422, "Validation failed");

    expect(error.status).toBe(422);
  });
});
