import { useState, useEffect } from "react";
import { apiFetch } from "../utils/api";
import type { Client } from "../types/invoice";

interface Props {
  value: string;
  onChange: (clientName: string, clientId?: string | null) => void;
}

export default function ClientSelector({ value, onChange }: Props) {
  const [clients, setClients] = useState<Client[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [newName, setNewName] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPhone, setNewPhone] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    apiFetch<Client[]>("/clients")
      .then((data) => setClients(data || []))
      .catch((err) => console.error("Failed to fetch clients:", err))
      .finally(() => setLoading(false));
  }, []);

  const handleSelect = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const selectedId = e.target.value;
    if (selectedId === "__add__") {
      setShowAdd(true);
      return;
    }
    if (!selectedId) {
      onChange("");
      return;
    }
    const client = clients.find((c) => c.id === selectedId);
    if (client) {
      onChange(client.name, client.id);
    }
  };

  const handleAddClient = async () => {
    if (!newName.trim()) return;
    setSaving(true);
    try {
      const created = await apiFetch<Client>("/clients", {
        method: "POST",
        body: JSON.stringify({
          name: newName,
          email: newEmail,
          phone: newPhone,
          address: "",
        }),
      });
      setClients((prev) => [...prev, created]);
      onChange(created.name, created.id);
      setNewName("");
      setNewEmail("");
      setNewPhone("");
      setShowAdd(false);
    } catch (err) {
      console.error("Failed to create client:", err);
    } finally {
      setSaving(false);
    }
  };

  const selectedClient = clients.find((c) => c.name === value);

  return (
    <div>
      <label className="block text-sm font-medium text-gray-600 mb-1">
        Client
      </label>

      <select
        className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
        value={selectedClient?.id ?? (value ? "__manual__" : "")}
        onChange={handleSelect}
        disabled={loading}
      >
        <option value="">-- Select or type below --</option>
        {clients.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
        <option value="__add__">+ Add new client...</option>
      </select>

      {/* Manual name input (always shown) */}
      <input
        className="mt-2 w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
        value={value}
        onChange={(e) => onChange(e.target.value, selectedClient?.id ?? null)}
        placeholder="Or type client name manually"
      />

      {/* Inline add form */}
      {showAdd && (
        <div className="mt-3 rounded border border-blue-200 bg-blue-50 p-4 space-y-3">
          <p className="text-sm font-medium text-blue-800">Add New Client</p>
          <input
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="Client name *"
          />
          <input
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            value={newEmail}
            onChange={(e) => setNewEmail(e.target.value)}
            placeholder="Email (optional)"
          />
          <input
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            value={newPhone}
            onChange={(e) => setNewPhone(e.target.value)}
            placeholder="Phone (optional)"
          />
          <div className="flex gap-2">
            <button
              onClick={handleAddClient}
              disabled={saving || !newName.trim()}
              className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save Client"}
            </button>
            <button
              onClick={() => setShowAdd(false)}
              className="rounded border border-gray-300 px-4 py-2 text-sm text-gray-600 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
