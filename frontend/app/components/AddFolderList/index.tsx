import { useEffect, useState } from "react";
import { Loader2, MessagesSquare } from "lucide-react";

import Field from "~/components/Field";
import { ApiError } from "~/lib/api";
import { useCreateFolder, useRenameFolder } from "~/lib/folders";
import type { Folder } from "~/types/subscription";

type AddFolderListProps = {
  /** When set, the modal renames that folder instead of creating one. */
  folder?: Folder | null;
  onCancel?: () => void;
  onDone?: () => void;
};

const AddFolderList = ({ folder, onCancel, onDone }: AddFolderListProps) => {
  const [name, setName] = useState<string>(folder?.name ?? "");
  const [error, setError] = useState<string | null>(null);

  const createFolder = useCreateFolder();
  const renameFolder = useRenameFolder();
  const isPending = createFolder.isPending || renameFolder.isPending;

  // The modal stays mounted between openings, so the field has to follow
  // whichever folder was picked — or be cleared for a creation.
  useEffect(() => {
    setName(folder?.name ?? "");
    setError(null);
  }, [folder]);

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);

    const trimmed = name.trim();
    if (!trimmed) {
      setError("Please enter a name.");
      return;
    }

    try {
      if (folder) {
        await renameFolder.mutateAsync({ id: folder.id, name: trimmed });
      } else {
        await createFolder.mutateAsync(trimmed);
      }
      setName("");
      onDone?.();
    } catch (err) {
      setError(
        err instanceof ApiError
          ? (Object.values(err.fieldErrors ?? {})[0] ?? err.message)
          : "The folder could not be saved.",
      );
    }
  };

  return (
    <form
      className="p-12 max-lg:px-8 max-md:pt-6 max-md:px-5 max-md:pb-6"
      onSubmit={onSubmit}
    >
      <div className="mb-8 h4">{folder ? "Rename folder" : "Add folder"}</div>
      <div className="relative z-10 mb-8">
        <Field
          label="Name"
          placeholder="Name"
          icon={MessagesSquare}
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={error ?? undefined}
          autoFocus
          required
        />
        {error && <div className="mt-2 caption1 text-red-500">{error}</div>}
      </div>

      <div className="flex justify-end">
        <button
          type="button"
          className="btn-stroke-light mr-3"
          onClick={onCancel}
          disabled={isPending}
        >
          Cancel
        </button>
        <button type="submit" className="btn-blue" disabled={isPending}>
          {isPending ? (
            <Loader2 size={18} className="animate-spin" />
          ) : folder ? (
            "Rename folder"
          ) : (
            "Add folder"
          )}
        </button>
      </div>
    </form>
  );
};

export default AddFolderList;
