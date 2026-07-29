import { useState } from "react";
import Field from "~/components/Field";
import Select from "~/components/Select";
import { MessagesSquare } from "lucide-react";

const colors = [
    {
        id: "0",
        title: "Chinese Violet",
        color: "#8C6584",
    },
    {
        id: "1",
        title: "Dodger blue",
        color: "#3E90F0",
    },
    {
        id: "2",
        title: "Golden Gate Bridge",
        color: "#D84C10",
    },
    {
        id: "3",
        title: "Veronica",
        color: "#8E55EA",
    },
    {
        id: "4",
        title: "Sugus green",
        color: "#7ECE18",
    },
];

type AddFolderListProps = {
    onCancel?: () => void;
};

const AddFolderList = ({ onCancel }: AddFolderListProps) => {
    const [name, setName] = useState<string>("");
    const [color, setColor] = useState<any>(colors[1]);

    return (
        <div className="p-12 max-lg:px-8 max-md:pt-6 max-md:px-5 max-md:pb-6">
            <div className="mb-8 h4">Add folder list</div>
            <div className="relative z-10 flex mb-8 max-md:block">
                <Field
                    className="grow mr-3 max-md:mr-0 max-md:mb-3"
                    label="Name"
                    placeholder="Name"
                    icon={MessagesSquare}
                    value={name}
                    onChange={(e: any) => setName(e.target.value)}
                    required
                />
                <Select
                    label="Color"
                    className="shrink-0 min-w-58"
                    items={colors}
                    value={color}
                    onChange={setColor}
                />
            </div>

            <div className="flex justify-end">
                <button className="btn-stroke-light mr-3" onClick={onCancel}>
                    Cancel
                </button>
                <button className="btn-blue">Add folder</button>
            </div>
        </div>
    );
};

export default AddFolderList;
