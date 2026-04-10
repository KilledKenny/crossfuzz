package crossfuzz;

import java.lang.instrument.ClassFileTransformer;
import java.security.ProtectionDomain;
import java.util.HashSet;
import java.util.Set;
import org.objectweb.asm.ClassReader;
import org.objectweb.asm.ClassWriter;
import org.objectweb.asm.Opcodes;
import org.objectweb.asm.tree.AbstractInsnNode;
import org.objectweb.asm.tree.ClassNode;
import org.objectweb.asm.tree.InsnList;
import org.objectweb.asm.tree.JumpInsnNode;
import org.objectweb.asm.tree.LabelNode;
import org.objectweb.asm.tree.LdcInsnNode;
import org.objectweb.asm.tree.LookupSwitchInsnNode;
import org.objectweb.asm.tree.MethodInsnNode;
import org.objectweb.asm.tree.MethodNode;
import org.objectweb.asm.tree.TableSwitchInsnNode;
import org.objectweb.asm.tree.TryCatchBlockNode;

public class CoverageTransformer implements ClassFileTransformer {

    private static final String[] EXCLUDED = {
        "crossfuzz/", "java/", "javax/", "sun/", "com/sun/",
        "jdk/", "org/objectweb/asm/"
    };

    @Override
    public byte[] transform(ClassLoader loader, String className,
            Class<?> classBeingRedefined, ProtectionDomain pd, byte[] buf) {
        if (className == null) return null;
        for (String p : EXCLUDED) {
            if (className.startsWith(p)) return null;
        }
        try {
            ClassReader cr = new ClassReader(buf);
            ClassNode cn = new ClassNode();
            cr.accept(cn, ClassReader.EXPAND_FRAMES);
            for (MethodNode mn : cn.methods) {
                instrumentMethod(className, mn);
            }
            ClassWriter cw = new ClassWriter(cr, ClassWriter.COMPUTE_MAXS);
            cn.accept(cw);
            return cw.toByteArray();
        } catch (Throwable t) {
            return null;
        }
    }

    private void instrumentMethod(String cls, MethodNode mn) {
        // Collect branch target labels (basic block entries)
        Set<LabelNode> targets = new HashSet<>();
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof JumpInsnNode) {
                targets.add(((JumpInsnNode) n).label);
            } else if (n instanceof TableSwitchInsnNode) {
                TableSwitchInsnNode ts = (TableSwitchInsnNode) n;
                targets.add(ts.dflt);
                ts.labels.forEach(targets::add);
            } else if (n instanceof LookupSwitchInsnNode) {
                LookupSwitchInsnNode ls = (LookupSwitchInsnNode) n;
                targets.add(ls.dflt);
                ls.labels.forEach(targets::add);
            }
        }
        for (TryCatchBlockNode tcb : mn.tryCatchBlocks) {
            targets.add(tcb.handler);
        }

        // Inject at method entry (before first instruction)
        mn.instructions.insert(makeHit(cls, mn.name, 0));

        // Inject after each branch-target label
        int blockId = 1;
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof LabelNode && targets.contains(n)) {
                mn.instructions.insert(n, makeHit(cls, mn.name, blockId++));
            }
        }
    }

    private InsnList makeHit(String cls, String method, int blockId) {
        int idx = (cls + "_" + method + "_" + blockId).hashCode() & 0xFFFF;
        InsnList l = new InsnList();
        l.add(new LdcInsnNode(idx));
        l.add(new MethodInsnNode(Opcodes.INVOKESTATIC,
            "crossfuzz/CoverageRuntime", "hit", "(I)V", false));
        return l;
    }
}
